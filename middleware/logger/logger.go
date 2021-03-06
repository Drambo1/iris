// Package logger provides request logging via middleware. See _examples/logging/request-logger
package logger

import (
	"fmt"
	"strconv"
	"time"

	"github.com/kataras/iris/v12/context"

	"github.com/ryanuber/columnize"
)

func init() {
	context.SetHandlerName("iris/middleware/logger.*", "iris.logger")
}

type requestLoggerMiddleware struct {
	config Config
}

// New creates and returns a new request logger middleware.
// Do not confuse it with the framework's Logger.
// This is for the http requests.
//
// Receives an optional configuation.
// Usage: app.UseRouter(logger.New()).
func New(cfg ...Config) context.Handler {
	c := DefaultConfig()
	if len(cfg) > 0 {
		c = cfg[0]
	}
	c.buildSkipper()
	l := &requestLoggerMiddleware{config: c}

	return l.ServeHTTP
}

func (l *requestLoggerMiddleware) getPath(ctx *context.Context) string {
	if l.config.Query {
		return ctx.Request().URL.RequestURI()
	}
	return ctx.Path()
}

// Serve serves the middleware
func (l *requestLoggerMiddleware) ServeHTTP(ctx *context.Context) {
	// skip logs and serve the main request immediately
	if l.config.skip != nil {
		if l.config.skip(ctx) {
			ctx.Next()
			return
		}
	}

	// all except latency to string
	var status, ip, method, path string
	var latency time.Duration
	var startTime, endTime time.Time
	startTime = time.Now()

	// Before Next.
	if l.config.IP {
		ip = ctx.RemoteAddr()
	}

	if l.config.Method {
		method = ctx.Method()
	}

	if l.config.Path {
		path = l.getPath(ctx)
	}

	ctx.Next()

	// no time.Since in order to format it well after
	endTime = time.Now()
	latency = endTime.Sub(startTime)

	if l.config.PathAfterHandler /* we don't care if Path is disabled */ {
		path = l.getPath(ctx)
	}

	if l.config.Status {
		status = strconv.Itoa(ctx.GetStatusCode())
	}

	var message interface{}
	if ctxKeys := l.config.MessageContextKeys; len(ctxKeys) > 0 {
		for _, key := range ctxKeys {
			msg := ctx.Values().Get(key)
			if message == nil {
				message = msg
			} else {
				message = fmt.Sprintf(" %v %v", message, msg)
			}
		}
	}
	var headerMessage interface{}
	if headerKeys := l.config.MessageHeaderKeys; len(headerKeys) > 0 {
		for _, key := range headerKeys {
			msg := ctx.GetHeader(key)
			if headerMessage == nil {
				headerMessage = msg
			} else {
				headerMessage = fmt.Sprintf(" %v %v", headerMessage, msg)
			}
		}
	}

	// print the logs
	if logFunc := l.config.LogFunc; logFunc != nil {
		logFunc(endTime, latency, status, ip, method, path, message, headerMessage)
		return
	} else if logFuncCtx := l.config.LogFuncCtx; logFuncCtx != nil {
		logFuncCtx(ctx, latency)
		return
	}

	if l.config.Columns {
		endTimeFormatted := endTime.Format("2006/01/02 - 15:04:05")
		output := Columnize(endTimeFormatted, latency, status, ip, method, path, message, headerMessage)
		_, _ = ctx.Application().Logger().Printer.Write([]byte(output))
		return
	}
	// no new line, the framework's logger is responsible how to render each log.
	line := fmt.Sprintf("%v %4v %s %s %s", status, latency, ip, method, path)
	if message != nil {
		line += fmt.Sprintf(" %v", message)
	}

	if headerMessage != nil {
		line += fmt.Sprintf(" %v", headerMessage)
	}

	if context.StatusCodeNotSuccessful(ctx.GetStatusCode()) {
		ctx.Application().Logger().Warn(line)
	} else {
		ctx.Application().Logger().Info(line)
	}

	if l.config.TraceRoute && ctx.GetCurrentRoute() != nil /* it is nil on unhandled error codes */ {
		// Get the total length of handlers and see if all are executed.
		// Note(@kataras): we get those after handler executed, because
		// filters (and overlap) feature will set the handlers on router build
		// state to fullfil their needs. And we need to respect
		// any dev's custom SetHandlers&Do actions too so we don't give false info.
		// if n, idx := len(ctx.Handlers()), ctx.HandlerIndex(-1); idx < n-1 {
		//
		// }
		// Let's pass it into the Trace function itself which will "mark"
		// every handler that is eventually executed.
		// Note that if StopExecution is called, the index is always -1,
		// so no "mark" signs will be printed at all <- this can be fixed by introducing a new ctx field.
		ctx.GetCurrentRoute().Trace(ctx.Application().Logger().Printer, ctx.HandlerIndex(-1))
	}
}

// Columnize formats the given arguments as columns and returns the formatted output,
// note that it appends a new line to the end.
func Columnize(nowFormatted string, latency time.Duration, status, ip, method, path string, message interface{}, headerMessage interface{}) string {
	titles := "Time | Status | Latency | IP | Method | Path"
	line := fmt.Sprintf("%s | %v | %4v | %s | %s | %s", nowFormatted, status, latency, ip, method, path)
	if message != nil {
		titles += " | Message"
		line += fmt.Sprintf(" | %v", message)
	}

	if headerMessage != nil {
		titles += " | HeaderMessage"
		line += fmt.Sprintf(" | %v", headerMessage)
	}

	outputC := []string{
		titles,
		line,
	}
	output := columnize.SimpleFormat(outputC) + "\n"
	return output
}
