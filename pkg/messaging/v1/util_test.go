// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1

import (
	"sort"
	"strings"
	"testing"

	internalv1pb "github.com/dapr/dapr/pkg/proto/daprinternal/v1"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/stretchr/testify/assert"
	epb "google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestInternalMetadataToHTTPHeader(t *testing.T) {
	testValue := &structpb.ListValue{
		Values: []*structpb.Value{
			{
				Kind: &structpb.Value_StringValue{StringValue: "fakeValue"},
			},
		},
	}

	fakeMetadata := map[string]*structpb.ListValue{
		"custom-header":  testValue,
		":method":        testValue,
		":scheme":        testValue,
		":path":          testValue,
		":authority":     testValue,
		"grpc-timeout":   testValue,
		"content-type":   testValue, // skip
		"grpc-trace-bin": testValue, // skip binary metadata
	}

	expectedKeyNames := []string{"custom-header", "dapr-method", "dapr-scheme", "dapr-path", "dapr-authority", "dapr-grpc-timeout"}
	savedHeaderKeyNames := []string{}
	InternalMetadataToHTTPHeader(fakeMetadata, func(k, v string) {
		savedHeaderKeyNames = append(savedHeaderKeyNames, k)
	})

	sort.Strings(expectedKeyNames)
	sort.Strings(savedHeaderKeyNames)

	assert.Equal(t, expectedKeyNames, savedHeaderKeyNames)
}

func TestGrpcMetadataToInternalMetadata(t *testing.T) {
	testMD := metadata.Pairs(
		"key", "key value",
		"key-bin", string([]byte{101, 200}),
	)
	internalMD := GrpcMetadataToInternalMetadata(testMD)

	assert.Equal(t, "key value", internalMD["key"].GetValues()[0].GetStringValue())
	assert.Equal(t, 1, len(internalMD["key"].GetValues()))

	assert.Equal(t, string([]byte{101, 200}), internalMD["key-bin"].GetValues()[0].GetStringValue(), "binary metadata must be saved")
	assert.Equal(t, 1, len(internalMD["key-bin"].GetValues()))
}

func TestIsJSONContentType(t *testing.T) {
	var contentTypeTests = []struct {
		in  string
		out bool
	}{
		{"application/json", true},
		{"text/plains; charset=utf-8", false},
		{"application/json; charset=utf-8", true},
	}

	for _, tt := range contentTypeTests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.out, IsJSONContentType(tt.in))
		})
	}
}

func TestInternalMetadataToGrpcMetadata(t *testing.T) {
	httpHeaders := map[string]*structpb.ListValue{
		"Host": {
			Values: []*structpb.Value{
				{Kind: &structpb.Value_StringValue{StringValue: "localhost"}},
			},
		},
		"Content-Type": {
			Values: []*structpb.Value{
				{Kind: &structpb.Value_StringValue{StringValue: "application/json"}},
			},
		},
		"Accept-Encoding": {
			Values: []*structpb.Value{
				{Kind: &structpb.Value_StringValue{StringValue: "gzip, deflate"}},
			},
		},
		"User-Agent": {
			Values: []*structpb.Value{
				{Kind: &structpb.Value_StringValue{StringValue: "Go-http-client/1.1"}},
			},
		},
	}

	t.Run("without http header conversion for http headers", func(t *testing.T) {
		convertedMD := InternalMetadataToGrpcMetadata(httpHeaders, false)
		assert.Equal(t, 4, convertedMD.Len())
		assert.Equal(t, "localhost", convertedMD["host"][0])
		assert.Equal(t, "application/json", convertedMD["content-type"][0])
		assert.Equal(t, "gzip, deflate", convertedMD["accept-encoding"][0])
		assert.Equal(t, "Go-http-client/1.1", convertedMD["user-agent"][0])
	})

	t.Run("with http header conversion for http headers", func(t *testing.T) {
		convertedMD := InternalMetadataToGrpcMetadata(httpHeaders, true)
		assert.Equal(t, 4, convertedMD.Len())
		assert.Equal(t, "localhost", convertedMD["dapr-host"][0])
		assert.Equal(t, "application/json", convertedMD["dapr-content-type"][0])
		assert.Equal(t, "gzip, deflate", convertedMD["accept-encoding"][0])
		assert.Equal(t, "Go-http-client/1.1", convertedMD["user-agent"][0])
	})

	grpcMetadata := map[string]*structpb.ListValue{
		":authority": {
			Values: []*structpb.Value{
				{Kind: &structpb.Value_StringValue{StringValue: "localhost"}},
			},
		},
		"grpc-timeout": {
			Values: []*structpb.Value{
				{Kind: &structpb.Value_StringValue{StringValue: "1S"}},
			},
		},
		"grpc-encoding": {
			Values: []*structpb.Value{
				{Kind: &structpb.Value_StringValue{StringValue: "gzip, deflate"}},
			},
		},
		"authorization": {
			Values: []*structpb.Value{
				{Kind: &structpb.Value_StringValue{StringValue: "bearer token"}},
			},
		},
		"grpc-trace-bin": {
			Values: []*structpb.Value{
				{Kind: &structpb.Value_StringValue{StringValue: string([]byte{10, 30, 50, 60})}},
			},
		},
		"my-metadata": {
			Values: []*structpb.Value{
				{Kind: &structpb.Value_StringValue{StringValue: "value1"}},
				{Kind: &structpb.Value_StringValue{StringValue: "value2"}},
				{Kind: &structpb.Value_StringValue{StringValue: "value3"}},
			},
		},
	}

	t.Run("with grpc header conversion for grpc headers", func(t *testing.T) {
		convertedMD := InternalMetadataToGrpcMetadata(grpcMetadata, true)
		assert.Equal(t, 5, convertedMD.Len())
		assert.Equal(t, "localhost", convertedMD[":authority"][0])
		assert.Equal(t, "1S", convertedMD["grpc-timeout"][0])
		assert.Equal(t, "gzip, deflate", convertedMD["grpc-encoding"][0])
		assert.Equal(t, "bearer token", convertedMD["authorization"][0])
		_, ok := convertedMD["grpc-trace-bin"]
		assert.False(t, ok)
		assert.Equal(t, "value1", convertedMD["my-metadata"][0])
		assert.Equal(t, "value2", convertedMD["my-metadata"][1])
		assert.Equal(t, "value3", convertedMD["my-metadata"][2])
	})
}

func TestErrorFromHTTPResponseCode(t *testing.T) {
	t.Run("OK", func(t *testing.T) {
		// act
		err := ErrorFromHTTPResponseCode(200, "OK")

		// assert
		assert.NoError(t, err)
	})

	t.Run("NotFound", func(t *testing.T) {
		// act
		err := ErrorFromHTTPResponseCode(404, "Not Found")

		// assert
		s, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.NotFound, s.Code())
		assert.Equal(t, "Not Found", s.Message())
		errInfo := (s.Details()[0]).(*epb.ErrorInfo)
		assert.Equal(t, "404", errInfo.GetMetadata()[errorInfoHTTPCodeMetadata])
		assert.Equal(t, "Not Found", errInfo.GetMetadata()[errorInfoHTTPErrorMetadata])
	})

	t.Run("Unknown", func(t *testing.T) {
		// act
		err := ErrorFromHTTPResponseCode(201, "Created")

		// assert
		s, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unknown, s.Code())
		assert.Equal(t, "Created", s.Message())
		errInfo := (s.Details()[0]).(*epb.ErrorInfo)
		assert.Equal(t, "201", errInfo.GetMetadata()[errorInfoHTTPCodeMetadata])
		assert.Equal(t, "Created", errInfo.GetMetadata()[errorInfoHTTPErrorMetadata])
	})

	t.Run("Internal Server Error", func(t *testing.T) {
		// act
		err := ErrorFromHTTPResponseCode(500, "HTTPExtensions is not given")

		// assert
		s, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unknown, s.Code())
		assert.Equal(t, "Internal Server Error", s.Message())
		errInfo := (s.Details()[0]).(*epb.ErrorInfo)
		assert.Equal(t, "500", errInfo.GetMetadata()[errorInfoHTTPCodeMetadata])
		assert.Equal(t, "HTTPExtensions is not given", errInfo.GetMetadata()[errorInfoHTTPErrorMetadata])
	})

	t.Run("Truncate error message", func(t *testing.T) {
		longMessage := strings.Repeat("test", 30)

		// act
		err := ErrorFromHTTPResponseCode(500, longMessage)

		// assert
		s, _ := status.FromError(err)
		errInfo := (s.Details()[0]).(*epb.ErrorInfo)
		assert.Equal(t, 63, len(errInfo.GetMetadata()[errorInfoHTTPErrorMetadata]))
	})
}

func TestErrorFromInternalStatus(t *testing.T) {
	expected := status.New(codes.Internal, "Internal Service Error")
	expected.WithDetails(
		&epb.DebugInfo{
			StackEntries: []string{
				"first stack",
				"second stack",
			},
		},
	)

	internal := &internalv1pb.Status{
		Code:    expected.Proto().Code,
		Message: expected.Proto().Message,
		Details: expected.Proto().Details,
	}

	// act
	statusError := ErrorFromInternalStatus(internal)

	// assert
	actual, ok := status.FromError(statusError)
	assert.True(t, ok)
	assert.Equal(t, expected, actual)
}
