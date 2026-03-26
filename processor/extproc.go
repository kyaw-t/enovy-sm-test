package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
)

type VertexProcessor struct {
	extprocv3.UnimplementedExternalProcessorServer
	secretCache    *SecretCache
	safetySettings []SafetySetting
}

func NewVertexProcessor(secretCache *SecretCache, safetySettings []SafetySetting) *VertexProcessor {
	return &VertexProcessor{
		secretCache:    secretCache,
		safetySettings: safetySettings,
	}
}

func (p *VertexProcessor) Process(stream extprocv3.ExternalProcessor_ProcessServer) error {
	ctx := stream.Context()

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		var resp *extprocv3.ProcessingResponse

		switch v := req.Request.(type) {
		case *extprocv3.ProcessingRequest_RequestHeaders:
			resp, err = p.handleRequestHeaders(ctx, v.RequestHeaders)
		case *extprocv3.ProcessingRequest_RequestBody:
			resp, err = p.handleRequestBody(ctx, v.RequestBody)
		default:
			// For any other phase, just continue
			resp = &extprocv3.ProcessingResponse{
				Response: &extprocv3.ProcessingResponse_RequestHeaders{
					RequestHeaders: &extprocv3.HeadersResponse{},
				},
			}
		}

		if err != nil {
			log.Printf("error processing request: %v", err)
			return err
		}

		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

// handleRequestHeaders injects the API key into the :path query string
// and tells envoy to also send us the request body (BUFFERED mode).
func (p *VertexProcessor) handleRequestHeaders(ctx context.Context, headers *extprocv3.HttpHeaders) (*extprocv3.ProcessingResponse, error) {
	apiKey, err := p.secretCache.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	// Find the current :path
	var currentPath string
	for _, h := range headers.Headers.Headers {
		if h.Key == ":path" {
			currentPath = h.RawValue
			if currentPath == "" {
				currentPath = h.Value
			}
			break
		}
	}

	// Append the API key as a query parameter
	if strings.Contains(currentPath, "?") {
		currentPath = currentPath + "&key=" + apiKey
	} else {
		currentPath = currentPath + "?key=" + apiKey
	}

	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_RequestHeaders{
			RequestHeaders: &extprocv3.HeadersResponse{
				Response: &extprocv3.CommonResponse{
					HeaderMutation: &extprocv3.HeaderMutation{
						SetHeaders: []*corev3.HeaderValueOption{
							{
								Header: &corev3.HeaderValue{
									Key:      ":path",
									RawValue: []byte(currentPath),
								},
							},
						},
						RemoveHeaders: []string{"Authorization"},
					},
				},
			},
		},
		// Tell envoy to send us the body in BUFFERED mode
		ModeOverride: &extprocv3.ProcessingMode{
			RequestBodyMode: extprocv3.ProcessingMode_BUFFERED,
		},
	}, nil
}

// handleRequestBody parses the JSON body and injects safetySettings.
func (p *VertexProcessor) handleRequestBody(ctx context.Context, body *extprocv3.HttpBody) (*extprocv3.ProcessingResponse, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(body.Body, &payload); err != nil {
		// If we can't parse the body, just pass it through
		log.Printf("warning: could not parse request body as JSON: %v", err)
		return &extprocv3.ProcessingResponse{
			Response: &extprocv3.ProcessingResponse_RequestBody{
				RequestBody: &extprocv3.BodyResponse{},
			},
		}, nil
	}

	// Only inject safetySettings if not already present
	if _, exists := payload["safetySettings"]; !exists && len(p.safetySettings) > 0 {
		payload["safetySettings"] = p.safetySettings
	}

	mutatedBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal mutated body: %w", err)
	}

	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_RequestBody{
			RequestBody: &extprocv3.BodyResponse{
				Response: &extprocv3.CommonResponse{
					BodyMutation: &extprocv3.BodyMutation{
						Mutation: &extprocv3.BodyMutation_Body{
							Body: mutatedBody,
						},
					},
				},
			},
		},
	}, nil
}
