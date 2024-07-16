package config

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/inconshreveable/log15"
	"gopkg.in/yaml.v3"

	po "golang.ngrok.com/ngrok/policy"
)

type policy po.Policy
type rule po.Rule
type action po.Action
type trafficPolicy string

// WithPolicyString configures this edge with the provided policy configuration
// passed as a json or yaml string and overwrites any previously-set traffic policy
// https://ngrok.com/docs/http/traffic-policy
func WithPolicyString(str string) interface {
	HTTPEndpointOption
	TLSEndpointOption
	TCPEndpointOption
} {
	if !isJsonString(str) && !isYamlStr(str) {
		panic(errors.New("provided string is neither valid JSON nor valid YAML"))
	}
	return trafficPolicy(str)
}

func (p trafficPolicy) ApplyTLS(opts *tlsOptions) {
	opts.TrafficPolicy = string(p)
}

func (p trafficPolicy) ApplyHTTP(opts *httpOptions) {
	opts.TrafficPolicy = string(p)
}

func (p trafficPolicy) ApplyTCP(opts *tcpOptions) {
	opts.TrafficPolicy = string(p)
}

func isJsonString(jsonStr string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(jsonStr), &js) == nil
}

func isYamlStr(yamlStr string) bool {
	var yml map[string]any
	return yaml.Unmarshal([]byte(yamlStr), &yml) == nil
}

// WithPolicy is deprecated, use WithPolicyString instead.
// https://ngrok.com/docs/http/traffic-policy/
func WithPolicy(p po.Policy) interface {
	HTTPEndpointOption
	TLSEndpointOption
	TCPEndpointOption
} {
	ret := policy(p)

	return &ret
}

func (p *policy) ApplyTLS(opts *tlsOptions) {
	opts.TrafficPolicy = policyToString(p)
}

func (p *policy) ApplyHTTP(opts *httpOptions) {
	opts.TrafficPolicy = policyToString(p)
}

func (p *policy) ApplyTCP(opts *tcpOptions) {
	opts.TrafficPolicy = policyToString(p)
}

// policyToString converts the policy into a JSON string representation. This is to help remap Policy to TrafficPolicy.
func policyToString(p *policy) string {
	logger := log15.New()
	logger.Warn("WithPolicy has been deprecated. Please use WithPolicyString instead, as WithPolicy will stop working soon.")

	val, err := json.Marshal(p)
	if err != nil {
		panic(errors.New(fmt.Sprintf("failed to parse action configuration due to error: %s", err.Error())))
	}

	return string(val)
}
