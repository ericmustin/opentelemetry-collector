// Copyright 2020, OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package datadogexporter

import (
	"context"
	"fmt"

	// "github.com/DataDog/opencensus-go-exporter-datadog"
	"go.uber.org/zap"
	// "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"

	"go.opentelemetry.io/collector/component"
	// "go.opentelemetry.io/collector/component/componenterror"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/consumer/pdata"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/translator/internaldata"
)

// newTraceExporter return a new Datadog trace exporter.
func newDatadogTraceExporter(cfg configmodels.Exporter, logger *zap.Logger) (component.TraceExporter, error) {

	l := &traceSender{
		logger: logger,
	}

	// var err error
	// if l.client, err = NewLogServiceClient(cfg.(*Config), logger); err != nil {
	// 	return nil, err
	// }

	return exporterhelper.NewTraceExporter(
		cfg,
		l.pushTraceData)
}

type traceSender struct {
	logger *zap.Logger
	// client LogServiceClient
}

func (s *traceSender) pushTraceData(
	_ context.Context,
	td pdata.Traces,
) (int, error) {
	octds := internaldata.TraceDataToOC(td)
	ddTraces := make([][]*ddSpan, 0, len(octds))
	// var errs []error

	for _, octd := range octds {
		ddTrace := make([]*ddSpan, 0)

		for _, span := range octd.Spans {
			ddspan := convertSpan(span)
			ddTrace = append(ddTrace, ddspan)
		}

		ddTraces = append(ddTraces, ddTrace)

	}

	// logger.Info(ddTraces)
	fmt.Printf("%v", ddTraces)

	return len(ddTraces), nil
}
