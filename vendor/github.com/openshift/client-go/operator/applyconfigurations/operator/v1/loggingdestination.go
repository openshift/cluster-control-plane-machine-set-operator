// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1

import (
	operatorv1 "github.com/openshift/api/operator/v1"
)

// LoggingDestinationApplyConfiguration represents a declarative configuration of the LoggingDestination type for use
// with apply.
type LoggingDestinationApplyConfiguration struct {
	Type      *operatorv1.LoggingDestinationType                       `json:"type,omitempty"`
	Syslog    *SyslogLoggingDestinationParametersApplyConfiguration    `json:"syslog,omitempty"`
	Container *ContainerLoggingDestinationParametersApplyConfiguration `json:"container,omitempty"`
}

// LoggingDestinationApplyConfiguration constructs a declarative configuration of the LoggingDestination type for use with
// apply.
func LoggingDestination() *LoggingDestinationApplyConfiguration {
	return &LoggingDestinationApplyConfiguration{}
}

// WithType sets the Type field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Type field is set to the value of the last call.
func (b *LoggingDestinationApplyConfiguration) WithType(value operatorv1.LoggingDestinationType) *LoggingDestinationApplyConfiguration {
	b.Type = &value
	return b
}

// WithSyslog sets the Syslog field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Syslog field is set to the value of the last call.
func (b *LoggingDestinationApplyConfiguration) WithSyslog(value *SyslogLoggingDestinationParametersApplyConfiguration) *LoggingDestinationApplyConfiguration {
	b.Syslog = value
	return b
}

// WithContainer sets the Container field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Container field is set to the value of the last call.
func (b *LoggingDestinationApplyConfiguration) WithContainer(value *ContainerLoggingDestinationParametersApplyConfiguration) *LoggingDestinationApplyConfiguration {
	b.Container = value
	return b
}
