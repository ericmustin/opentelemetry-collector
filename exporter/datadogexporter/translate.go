// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadog.com/).
// Copyright 2018 Datadog, Inc.

package datadogexporter

import (
	"encoding/binary"
	"fmt"
	"net/http"
	"strconv"

	// "github.com/DataDog/opencensus-go-exporter-datadog"
	octrace "github.com/census-instrumentation/opencensus-proto/gen-go/trace/v1"
	"go.opencensus.io/trace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	// "go.opentelemetry.io/collector/component"
	// "go.opentelemetry.io/collector/component/componenterror"
	// "go.opentelemetry.io/collector/config/configmodels"
	// "go.opentelemetry.io/collector/consumer/pdata"
	// "go.opentelemetry.io/collector/exporter/exporterhelper"
	// "go.opentelemetry.io/collector/translator/internaldata"
)

// statusCodes maps (*trace.SpanData).Status.Code to their message and http status code. See:
// https://github.com/googleapis/googleapis/blob/master/google/rpc/code.proto.
var statusCodes = map[int32]codeDetails{
	trace.StatusCodeOK:                 {message: "OK", status: http.StatusOK},
	trace.StatusCodeCancelled:          {message: "CANCELLED", status: 499},
	trace.StatusCodeUnknown:            {message: "UNKNOWN", status: http.StatusInternalServerError},
	trace.StatusCodeInvalidArgument:    {message: "INVALID_ARGUMENT", status: http.StatusBadRequest},
	trace.StatusCodeDeadlineExceeded:   {message: "DEADLINE_EXCEEDED", status: http.StatusGatewayTimeout},
	trace.StatusCodeNotFound:           {message: "NOT_FOUND", status: http.StatusNotFound},
	trace.StatusCodeAlreadyExists:      {message: "ALREADY_EXISTS", status: http.StatusConflict},
	trace.StatusCodePermissionDenied:   {message: "PERMISSION_DENIED", status: http.StatusForbidden},
	trace.StatusCodeResourceExhausted:  {message: "RESOURCE_EXHAUSTED", status: http.StatusTooManyRequests},
	trace.StatusCodeFailedPrecondition: {message: "FAILED_PRECONDITION", status: http.StatusBadRequest},
	trace.StatusCodeAborted:            {message: "ABORTED", status: http.StatusConflict},
	trace.StatusCodeOutOfRange:         {message: "OUT_OF_RANGE", status: http.StatusBadRequest},
	trace.StatusCodeUnimplemented:      {message: "UNIMPLEMENTED", status: http.StatusNotImplemented},
	trace.StatusCodeInternal:           {message: "INTERNAL", status: http.StatusInternalServerError},
	trace.StatusCodeUnavailable:        {message: "UNAVAILABLE", status: http.StatusServiceUnavailable},
	trace.StatusCodeDataLoss:           {message: "DATA_LOSS", status: http.StatusNotImplemented},
	trace.StatusCodeUnauthenticated:    {message: "UNAUTHENTICATED", status: http.StatusUnauthorized},
}

// codeDetails specifies information about a trace status code.
type codeDetails struct {
	message string // status message
	status  int    // corresponding HTTP status code
}

// ddSpan represents the Datadog span definition.
type ddSpan struct {
	SpanID   uint64             `msg:"span_id"`
	TraceID  uint64             `msg:"trace_id"`
	ParentID uint64             `msg:"parent_id"`
	Name     string             `msg:"name"`
	Service  string             `msg:"service"`
	Resource string             `msg:"resource"`
	Type     string             `msg:"type"`
	Start    int64              `msg:"start"`
	Duration int64              `msg:"duration"`
	Meta     map[string]string  `msg:"meta,omitempty"`
	Metrics  map[string]float64 `msg:"metrics,omitempty"`
	Error    int32              `msg:"error"`
}

// convertSpan takes an OpenCensus span and returns a Datadog span.
func convertSpan(s *octrace.Span) *ddSpan {
	startNano := s.StartTime
	span := &ddSpan{
		TraceID:  binary.BigEndian.Uint64(s.TraceId[8:]),
		SpanID:   binary.BigEndian.Uint64(s.SpanId),
		Name:     "opentelemetry",
		Resource: s.Name.Value,
		Service:  "service_example",
		Start:    int64(startNano.GetNanos()),
		Duration: int64(s.EndTime.GetNanos()) - int64(startNano.GetNanos()),
		Metrics:  map[string]float64{},
		Meta:     map[string]string{},
	}

	span.ParentID = binary.BigEndian.Uint64(s.ParentSpanId)

	code, ok := statusCodes[s.Status.Code]
	if !ok {
		code = codeDetails{
			message: "ERR_CODE_" + strconv.FormatInt(int64(s.Status.Code), 10),
			status:  http.StatusInternalServerError,
		}
	}

	switch s.Kind {
	case trace.SpanKindClient:
		span.Type = "client"
		if code.status/100 == 4 {
			span.Error = 1
		}
	case trace.SpanKindServer:
		span.Type = "server"
		fallthrough
	default:
		if code.status/100 == 5 {
			span.Error = 1
		}
	}

	if span.Error == 1 {
		span.Meta[ext.ErrorType] = code.message
		if msg := s.Status.Message; msg != "" {
			span.Meta[ext.ErrorMsg] = msg
		}
	}

	span.Meta[keyStatusCode] = strconv.Itoa(int(s.Status.Code))
	span.Meta[keyStatus] = code.message
	if msg := s.Status.Message; msg != "" {
		span.Meta[keyStatusDescription] = msg
	}

	// for key, val := range e.opts.GlobalTags {
	// 	setTag(span, key, val)
	// }
	for key, val := range s.GetAttributes().GetAttributeMap() {
		setTag(span, key, val)
	}
	return span
}

const (
	keySamplingPriority  = "_sampling_priority_v1"
	keyStatusDescription = "opencensus.status_description"
	keyStatusCode        = "opencensus.status_code"
	keyStatus            = "opencensus.status"
	keySpanName          = "span.name"
	// keySamplingPriorityRate = "_sampling_priority_rate_v1"
)

func setTag(s *ddSpan, key string, val interface{}) {
	if key == ext.Error {
		setError(s, val)
		return
	}
	switch v := val.(type) {
	case string:
		setStringTag(s, key, v)
	case bool:
		if v {
			setStringTag(s, key, "true")
		} else {
			setStringTag(s, key, "false")
		}
	case float64:
		setMetric(s, key, v)
	case int64:
		setMetric(s, key, float64(v))
	default:
		// should never happen according to docs, nevertheless
		// we should account for this to avoid exceptions
		setStringTag(s, key, fmt.Sprintf("%v", v))
	}
}

func setMetric(s *ddSpan, key string, v float64) {
	switch key {
	case ext.SamplingPriority:
		s.Metrics[keySamplingPriority] = v
	default:
		s.Metrics[key] = v
	}
}

func setStringTag(s *ddSpan, key, v string) {
	switch key {
	case ext.ServiceName:
		s.Service = v
	case ext.ResourceName:
		s.Resource = v
	case ext.SpanType:
		s.Type = v
	case ext.AnalyticsEvent:
		if v != "false" {
			setMetric(s, ext.EventSampleRate, 1)
		} else {
			setMetric(s, ext.EventSampleRate, 0)
		}
	case keySpanName:
		s.Name = v
	default:
		s.Meta[key] = v
	}
}

func setError(s *ddSpan, val interface{}) {
	switch v := val.(type) {
	case string:
		s.Error = 1
		s.Meta[ext.ErrorMsg] = v
	case bool:
		if v {
			s.Error = 1
		} else {
			s.Error = 0
		}
	case int64:
		if v > 0 {
			s.Error = 1
		} else {
			s.Error = 0
		}
	case nil:
		s.Error = 0
	default:
		s.Error = 1
	}
}
