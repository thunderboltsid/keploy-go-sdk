package integrations

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/bnkamalesh/webgo/v4"
	"io"
	"io/ioutil"
	"strings"

	// "log"
	"net"
	"net/http"

	// "net/url"
	"os"
	"time"

	// "github.com/bnkamalesh/webgo/v4/middleware/accesslog"
	// "github.com/bnkamalesh/webgo/v4/middleware/cors"
	"github.com/keploy/go-sdk/keploy"
)

func WebGoV4(app *keploy.App, w *webgo.Router) {
	mode := os.Getenv("KEPLOY_SDK_MODE")
	switch mode {
	case "test":
		w.Use(testMWWebGoV4(app))
		go app.Test()
	case "off":
		// dont run the SDK
	default:
		w.Use(captureMWWebGoV4(app))
	}
}

func testMWWebGoV4(app *keploy.App) func(http.ResponseWriter, *http.Request, http.HandlerFunc) {
	fmt.Println("TEST Mode")
	if nil == app {
		return func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
			next(w, r)
		}
	}
	return func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		id := r.Header.Get("KEPLOY_TEST_ID")
		if id == "" {
			next(w, r)
		}
		tc := app.Get(id)
		if tc == nil {
			next(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), keploy.KCTX, &keploy.Context{
			Mode:   "test",
			TestID: id,
			Deps:   tc.Deps,
		})
		r = r.WithContext(ctx)
		resp := captureRespWebGoV4(w, r, next)
		app.Resp[id] = resp

	}
}

func captureRespWebGoV4(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) keploy.HttpResp {
	resBody := new(bytes.Buffer)
	mw := io.MultiWriter(w, resBody)
	writer := &bodyDumpResponseWriterWebgoV4{Writer: mw, ResponseWriter: w, status: http.StatusOK}
	w = writer

	next(w, r)
	return keploy.HttpResp{
		//Status

		StatusCode: writer.status,
		Header:     w.Header(),
		Body:       resBody.String(),
	}
}

func captureMWWebGoV4(app *keploy.App) func(http.ResponseWriter, *http.Request, http.HandlerFunc) {
	if nil == app {
		return func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
			next(w, r)
		}
	}
	return func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {

		ctx := context.WithValue(r.Context(), keploy.KCTX, &keploy.Context{
			Mode: "capture",
		})

		r = r.WithContext(ctx)

		id := r.Header.Get("KEPLOY_TEST_ID")
		if id != "" {
			// id is only present during simulation
			// run it similar to how testcases would run
			ctx := context.WithValue(r.Context(), keploy.KCTX, &keploy.Context{
				Mode:   "test",
				TestID: id,
				Deps:   app.Deps[id],
			})

			r = r.WithContext(ctx)
			resp := captureRespWebGoV4(w, r, next)
			app.Resp[id] = resp
			return
		}
		// Request
		var reqBody []byte
		var err error
		if r.Body != nil { // Read
			reqBody, err = ioutil.ReadAll(r.Body)
			if err != nil {
				// TODO right way to log errors
				return
			}
		}
		r.Body = ioutil.NopCloser(bytes.NewBuffer(reqBody)) // Reset

		// Response
		resBody := new(bytes.Buffer)
		mw := io.MultiWriter(w, resBody)
		writer := &bodyDumpResponseWriterWebgoV4{Writer: mw, ResponseWriter: w, status: http.StatusOK}
		w = writer

		next(w, r)

		d := r.Context().Value(keploy.KCTX)
		if d == nil {
			app.Log.Error("failed to get keploy context")
			return
		}
		deps := d.(*keploy.Context)

		// u := &url.URL{
		// 	Scheme: r.URL.Scheme,
		// 	//User:     url.UserPassword("me", "pass"),
		// 	Host:     r.URL.Host,
		// 	Path:     r.URL.Path,
		// 	RawQuery: r.URL.RawQuery,
		// }
		app.Capture(keploy.TestCaseReq{
			Captured: time.Now().Unix(),
			AppID:    app.Name,
			URI:      urlPathWebGo(r.URL.Path, webgo.Context(r).Params()),
			HttpReq: keploy.HttpReq{
				Method:     keploy.Method(r.Method),
				ProtoMajor: r.ProtoMajor,
				ProtoMinor: r.ProtoMinor,
				URL:        r.URL.String(),
				URLParams:  urlParamsWebGoV4(r),
				Header:     r.Header,
				Body:       string(reqBody),
			},
			HttpResp: keploy.HttpResp{
				//Status
				StatusCode: writer.status,
				Header:     w.Header(),
				Body:       resBody.String(),
			},
			Deps: deps.Deps,
		})

	}
}

func urlParamsWebGoV4(r *http.Request) map[string]string {
	result := webgo.Context(r).Params()
	qp := r.URL.Query()
	for i, j := range qp {
		var s string
		if _, ok := result[i]; ok {
			s = result[i]
		}
		for _, e := range j {
			if s != "" {
				s += ", " + e
			} else {
				s = e
			}
		}
		result[i] = s
	}
	return result
}

func urlPathWebGo(url string, params map[string]string) string {
	res := url
	for i, j := range params {
		res = strings.Replace(url, j, ":"+i, -1)
	}
	return res
}

type bodyDumpResponseWriterWebgoV4 struct {
	io.Writer
	http.ResponseWriter
	status int
}

func (w *bodyDumpResponseWriterWebgoV4) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *bodyDumpResponseWriterWebgoV4) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func (w *bodyDumpResponseWriterWebgoV4) Flush() {
	w.ResponseWriter.(http.Flusher).Flush()
}

func (w *bodyDumpResponseWriterWebgoV4) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.ResponseWriter.(http.Hijacker).Hijack()
}