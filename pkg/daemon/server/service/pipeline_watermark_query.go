// Package service is built for querying metadata and to expose it over daemon service.
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/numaproj/numaflow/pkg/apis/numaflow/v1alpha1"
	"github.com/numaproj/numaflow/pkg/apis/proto/daemon"
	"github.com/numaproj/numaflow/pkg/isbsvc"
	jsclient "github.com/numaproj/numaflow/pkg/shared/clients/jetstream"
	"github.com/numaproj/numaflow/pkg/shared/logging"
	"github.com/numaproj/numaflow/pkg/watermark/fetch"
	"github.com/numaproj/numaflow/pkg/watermark/generic"
	"github.com/numaproj/numaflow/pkg/watermark/store/jetstream"
)

// watermarkFetchers used to store watermark metadata for propagation
type watermarkFetchers struct {
	fetchMap           map[string]fetch.Fetcher
	isWatermarkEnabled bool
}

// newVertexWatermarkFetcher creates a new instance of watermarkFetchers. This is used to populate a map of vertices to
// corresponding fetchers. These fetchers are tied to the incoming edge buffer of the current vertex (Vn), and read the
// watermark propagated by the vertex (Vn-1). As each vertex has one incoming edge, for the input vertex we read the source
// data buffer.
func newVertexWatermarkFetcher(pipeline *v1alpha1.Pipeline) (*watermarkFetchers, error) {

	// TODO: Return err instead of logging (https://github.com/numaproj/numaflow/pull/120#discussion_r927271677)
	ctx := context.Background()
	var wmFetcher = new(watermarkFetchers)
	var fromBufferName string

	wmFetcher.isWatermarkEnabled = pipeline.Spec.Watermark.Propagate
	if !wmFetcher.isWatermarkEnabled {
		return wmFetcher, nil
	}

	vertexWmMap := make(map[string]fetch.Fetcher)
	pipelineName := pipeline.Name

	// TODO: https://github.com/numaproj/numaflow/pull/120#discussion_r927316015
	for _, vertex := range pipeline.Spec.Vertices {
		// TODO: Checking if Vertex is source
		if vertex.Source != nil {
			fromBufferName = v1alpha1.GenerateSourceBufferName(pipeline.Namespace, pipelineName, vertex.Name)
		} else {
			// Currently we support only one incoming edge
			edge := pipeline.GetFromEdges(vertex.Name)[0]
			fromBufferName = v1alpha1.GenerateEdgeBufferName(pipeline.Namespace, pipelineName, edge.From, edge.To)
		}
		// Adding an extra entry for the sink vertex to check the watermark value progressed out of the vertex.
		// Can be checked by querying sinkName_SINK in the service
		if vertex.Sink != nil {
			toBufferName := v1alpha1.GenerateSinkBufferName(pipeline.Namespace, pipelineName, vertex.Name)
			fetchWatermark, err := createWatermarkFetcher(ctx, pipelineName, toBufferName, vertex.Name)
			if err != nil {
				return nil, fmt.Errorf("failed to create watermark fetcher  %w", err)
			}
			sinkVertex := vertex.Name + "_SINK"
			vertexWmMap[sinkVertex] = fetchWatermark
		}
		fetchWatermark, err := createWatermarkFetcher(ctx, pipelineName, fromBufferName, vertex.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to create watermark fetcher  %w", err)
		}
		vertexWmMap[vertex.Name] = fetchWatermark
	}
	wmFetcher.fetchMap = vertexWmMap
	return wmFetcher, nil
}

func createWatermarkFetcher(ctx context.Context, pipelineName string, fromBufferName string, vertexName string) (fetch.Fetcher, error) {
	hbBucket := isbsvc.JetStreamProcessorBucket(pipelineName, fromBufferName)
	hbWatch, err := jetstream.NewKVJetStreamKVWatch(ctx, pipelineName, hbBucket, jsclient.NewInClusterJetStreamClient())
	if err != nil {
		return nil, err
	}
	otBucket := isbsvc.JetStreamOTBucket(pipelineName, fromBufferName)
	otWatch, err := jetstream.NewKVJetStreamKVWatch(ctx, pipelineName, otBucket, jsclient.NewInClusterJetStreamClient())
	if err != nil {
		return nil, err
	}
	var fetchWmWatchers = generic.BuildFetchWMWatchers(hbWatch, otWatch)
	fetchWatermark := generic.NewGenericFetch(ctx, vertexName, fetchWmWatchers)
	return fetchWatermark, nil
}

// GetVertexWatermark is used to return the head watermark for a given vertex.
func (ps *pipelineMetadataQuery) GetVertexWatermark(ctx context.Context, request *daemon.GetVertexWatermarkRequest) (*daemon.GetVertexWatermarkResponse, error) {
	log := logging.FromContext(ctx)
	resp := new(daemon.GetVertexWatermarkResponse)
	vertexName := request.GetVertex()
	retFalse := false
	retTrue := true

	// If watermark is not enabled, return time zero
	if !ps.vertexWatermark.isWatermarkEnabled {
		timeZero := time.Unix(0, 0).Unix()
		v := &daemon.VertexWatermark{
			Pipeline:           &ps.pipeline.Name,
			Vertex:             request.Vertex,
			Watermark:          &timeZero,
			IsWatermarkEnabled: &retFalse,
		}
		resp.VertexWatermark = v
		return resp, nil
	}

	// Watermark is enabled
	vertexFetcher, ok := ps.vertexWatermark.fetchMap[vertexName]
	// Vertex not found
	if !ok {
		log.Errorf("watermark fetcher not available for vertex %s in the fetcher map", vertexName)
		return nil, fmt.Errorf("watermark not available for given vertex, %s", vertexName)
	}
	vertexWatermark := time.Time(vertexFetcher.GetHeadWatermark()).Unix()
	v := &daemon.VertexWatermark{
		Pipeline:           &ps.pipeline.Name,
		Vertex:             request.Vertex,
		Watermark:          &vertexWatermark,
		IsWatermarkEnabled: &retTrue,
	}
	resp.VertexWatermark = v
	return resp, nil
}
