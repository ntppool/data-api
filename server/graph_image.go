package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/contrib/instrumentation/net/http/httptrace/otelhttptrace"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
)

func (srv *Server) graphImage(c echo.Context) error {
	log := logger.Setup()
	ctx, span := tracing.Tracer().Start(c.Request().Context(), "graphImage")
	defer span.End()

	// cache errors briefly
	c.Response().Header().Set("Cache-Control", "public,max-age=240")

	serverID := c.Param("server")
	imageType := c.Param("type")
	log = log.With("serverID", serverID).With("type", imageType)
	log.InfoContext(ctx, "graph parameters")

	span.SetAttributes(attribute.String("url.server_parameter", serverID))

	if imageType != "offset.png" {
		return c.String(http.StatusNotFound, "invalid image name")
	}

	if len(c.QueryString()) > 0 {
		// people breaking the varnish cache by adding query parameters
		redirectURL := c.Request().URL
		redirectURL.RawQuery = ""
		log.InfoContext(ctx, "redirecting", "url", redirectURL.String())
		return c.Redirect(308, redirectURL.String())
	}

	serverData, err := srv.FindServer(ctx, serverID)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusInternalServerError, "server error")
	}
	if serverData.ID == 0 {
		return c.String(http.StatusNotFound, "not found")
	}
	if serverData.DeletionAge(7 * 24 * time.Hour) {
		return c.String(http.StatusNotFound, "not found")
	}

	if serverData.Ip != serverID {
		return c.Redirect(308, fmt.Sprintf("/graph/%s/offset.png", serverData.Ip))
	}

	contentType, data, err := srv.fetchGraph(ctx, serverData.Ip)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusInternalServerError, "server error")

	}
	if len(data) == 0 {
		span.RecordError(fmt.Errorf("no data"))
		return c.String(http.StatusInternalServerError, "server error")
	}

	ttl := 1800
	c.Response().Header().Set("Cache-Control",
		fmt.Sprintf("public,max-age=%d,s-maxage=%.0f",
			ttl, float64(ttl)*0.75,
		),
	)

	return c.Blob(http.StatusOK, contentType, data)
}

func (srv *Server) fetchGraph(ctx context.Context, serverIP string) (string, []byte, error) {
	log := logger.Setup()
	ctx, span := tracing.Tracer().Start(ctx, "fetchGraph")
	defer span.End()

	// q := url.Values{}
	// q.Set("graph_only", "1")
	// pagePath := srv.config.WebURL("/scores/" + serverIP, q)

	serviceHost := os.Getenv("screensnap_service")
	if len(serviceHost) == 0 {
		serviceHost = "screensnap"
	}

	reqURL := url.URL{
		Scheme: "http",
		Host:   serviceHost,
		Path:   fmt.Sprintf("/image/offset/%s", serverIP),
	}

	client := retryablehttp.NewClient()
	client.Logger = log
	client.HTTPClient.Transport = otelhttp.NewTransport(
		client.HTTPClient.Transport,
		otelhttp.WithClientTrace(func(ctx context.Context) *httptrace.ClientTrace {
			return otelhttptrace.NewClientTrace(ctx)
		}),
	)

	req, err := retryablehttp.NewRequestWithContext(ctx, "GET", reqURL.String(), nil)
	if err != nil {
		return "", nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		span.AddEvent("unexpected status code", trace.WithAttributes(attribute.Int64("http.status", int64(resp.StatusCode))))
		return "text/plain", nil, fmt.Errorf("upstream error %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}

	return resp.Header.Get("Content-Type"), b, nil
}

// # my $data = JSON::encode_json(
// #     {   url              => $url->as_string(),
// #         timeout          => 10,
// #         viewport         => "501x233",
// #         height           => 233,
// #         resource_timeout => 5,
// #         wait             => 0.5,
// #         scale_method     => "vector",
// #     }
// # );
