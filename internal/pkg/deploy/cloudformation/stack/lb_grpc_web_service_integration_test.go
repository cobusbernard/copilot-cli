// +build localintegration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package stack_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/aws/copilot-cli/internal/pkg/deploy/cloudformation/stack"
	"github.com/aws/copilot-cli/internal/pkg/template"

	"github.com/aws/copilot-cli/internal/pkg/manifest"

	"github.com/stretchr/testify/require"
)

const (
	svcGrpcManifestPath = "svc-grpc-manifest.yml"
)

func TestGrpcLoadBalancedWebService_Template(t *testing.T) {
	testCases := map[string]struct {
		envName       string
		svcStackPath  string
		svcParamsPath string
	}{
		"default env": {
			envName:       "test",
			svcStackPath:  "svc-grpc-test.stack.yml",
			svcParamsPath: "svc-grpc-test.params.json",
		},
	}
	path := filepath.Join("testdata", "workloads", svcGrpcManifestPath)
	manifestBytes, err := ioutil.ReadFile(path)
	require.NoError(t, err)
	for name, tc := range testCases {
		interpolated, err := manifest.NewInterpolator(appName, tc.envName).Interpolate(string(manifestBytes))
		require.NoError(t, err)
		mft, err := manifest.UnmarshalWorkload([]byte(interpolated))
		require.NoError(t, err)
		envMft, err := mft.ApplyEnv(tc.envName)
		require.NoError(t, err)

		err = envMft.Validate()
		require.NoError(t, err)

		v, ok := envMft.(*manifest.LoadBalancedWebService)
		require.True(t, ok)

		svcDiscoveryEndpointName := fmt.Sprintf("%s.%s.local", tc.envName, appName)

		serializer, err := stack.NewLoadBalancedWebService(v, tc.envName, appName, stack.RuntimeConfig{
			ServiceDiscoveryEndpoint: svcDiscoveryEndpointName,
			AccountID:                "123456789123",
			Region:                   "us-west-2",
		}, stack.WithHTTPS())

		tpl, err := serializer.Template()
		require.NoError(t, err, "template should render")
		regExpGUID := regexp.MustCompile(`([a-f\d]{8}-)([a-f\d]{4}-){3}([a-f\d]{12})`) // Matches random guids
		testName := fmt.Sprintf("CF Template should be equal/%s", name)
		parser := template.New()
		envController, err := parser.Read(envControllerPath)
		require.NoError(t, err)
		envControllerZipFile := envController.String()
		dynamicDesiredCount, err := parser.Read(dynamicDesiredCountPath)
		require.NoError(t, err)
		dynamicDesiredCountZipFile := dynamicDesiredCount.String()
		rulePriority, err := parser.Read(rulePriorityPath)
		require.NoError(t, err)
		rulePriorityZipFile := rulePriority.String()

		t.Run(testName, func(t *testing.T) {
			actualBytes := []byte(tpl)
			// Cut random GUID from template.
			actualBytes = regExpGUID.ReplaceAll(actualBytes, []byte("RandomGUID"))
			actualString := string(actualBytes)
			// Cut out zip file for more readable output
			actualString = strings.ReplaceAll(actualString, envControllerZipFile, "mockEnvControllerZipFile")
			actualString = strings.ReplaceAll(actualString, dynamicDesiredCountZipFile, "mockDynamicDesiredCountZipFile")
			actualString = strings.ReplaceAll(actualString, rulePriorityZipFile, "mockRulePriorityZipFile")

			actualBytes = []byte(actualString)
			mActual := make(map[interface{}]interface{})
			require.NoError(t, yaml.Unmarshal(actualBytes, mActual))

			expected, err := ioutil.ReadFile(filepath.Join("testdata", "workloads", tc.svcStackPath))
			require.NoError(t, err, "should be able to read expected bytes")
			expectedBytes := []byte(expected)
			mExpected := make(map[interface{}]interface{})
			require.NoError(t, yaml.Unmarshal(expectedBytes, mExpected))
			require.Equal(t, mExpected, mActual)
		})

		testName = fmt.Sprintf("Parameter values should render properly/%s", name)
		t.Run(testName, func(t *testing.T) {
			actualParams, err := serializer.SerializedParameters()
			require.NoError(t, err)

			path := filepath.Join("testdata", "workloads", tc.svcParamsPath)
			wantedCFNParamsBytes, err := ioutil.ReadFile(path)
			require.NoError(t, err)

			require.Equal(t, string(wantedCFNParamsBytes), actualParams)
		})
	}
}
