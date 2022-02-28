package v1beta1

import (
	configv1 "github.com/openshift/api/config/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ContrlPlaneMachineSet ensures that a specified number of control plane machine replicas are running at any given time.
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas
// +kubebuilder:printcolumn:name="Desired",type="integer",JSONPath=".spec.replicas",description="Desired Replicas"
// +kubebuilder:printcolumn:name="Current",type="integer",JSONPath=".status.replicas",description="Current Replicas"
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.readyReplicas",description="Ready Replicas"
// +kubebuilder:printcolumn:name="Updated",type="integer",JSONPath=".status.updatedReplicas",description="Updated Replicas"
// +kubebuilder:printcolumn:name="Unavailable",type="string",JSONPath=".status.unavailableReplicas",description="Observed number of unavailable replicas"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="ControlPlaneMachineSet age"
// Compatibility level 2: Stable within a major release for a minimum of 9 months or 3 minor releases (whichever is longer).
// +openshift:compatibility-gen:level=2
type ControlPlaneMachineSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ControlPlaneMachineSetSpec   `json:"spec,omitempty"`
	Status ControlPlaneMachineSetStatus `json:"status,omitempty"`
}

type ControlPlaneMachineSetSpec struct {
	// Replicas defines how many Control Plane Machines should be
	// created by this ControlPlaneMachineSet.
	// This field is immutable and cannot be changed after cluster
	// installation.
	// The ControlPlaneMachineSet only operates with 3 or 5 node control planes,
	// 3 and 5 are the only valid values for this field.
	// +kubebuilder:validation:Enum:=3;5
	// +kubebuilder:default:=3
	// +kubebuilder:validation:Required
	Replicas *int32 `json:"replicas"`

	// Strategy defines how the ControlPlaneMachineSet will update
	// Machines when it detects a change to the ProviderSpec.
	// +kubebuilder:default:={type: RollingUpdate}
	// +optional
	Strategy ControlPlaneMachineSetStrategy `json:"strategy,omitempty"`

	// FailureDomains is the list of failure domains (sometimes called
	// availability zones) in which the ControlPlaneMachineSet should balance
	// the Control Plane Machines.
	// This will be merged into the ProviderSpec given in the template.
	// This field is optional on platforms that do not require placement
	// information, eg OpenStack.
	// +optional
	FailureDomains FailureDomains `json:"failureDomains,omitempty"`

	// Label selector for Machines. Existing Machines selected by this
	// selector will be the ones affected by this ControlPlaneMachineSet.
	// It must match the template's labels.
	// This field is considered immutable after creation of the resource.
	// +kubebuilder:validation:Required
	Selector metav1.LabelSelector `json:"selector"`

	// Template describes the Control Plane Machines that will be created
	// by this ControlPlaneMachineSet.
	// +kubebuilder:validation:Required
	Template ControlPlaneMachineSetTemplate `json:"template"`
}

type ControlPlaneMachineSetTemplate struct {
	// ObjectMeta is the standard object metadata
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// Labels are required to match the ControlPlaneMachineSet selector.
	// +kubebuilder:validation:Required
	ObjectMeta metav1.ObjectMeta `json:"metadata"`

	// Spec contains the desired configuration of the Control Plane Machines.
	// The ProviderSpec within contains platform specific details
	// for creating the Control Plane Machines.
	// The ProviderSpec should be complete apart from the platform specific
	// failure domain field. This will be overriden when the Machines
	// are created based on the FailureDomains field.
	// +kubebuilder:validation:Required
	Spec machinev1beta1.MachineSpec `json:"spec"`
}

type ControlPlaneMachineSetStrategy struct {
	// Type defines the type of update strategy that should be
	// used when updating Machines owned by the ControlPlaneMachineSet.
	// Valid values are "RollingUpdate", "Recreate" and "OnDelete".
	// The current default value is "RollingUpdate".
	// +kubebuilder:default:="RollingUpdate"
	// +kubebuilder:validation:Enum:="RollingUpdate";"Recreate";"OnDelete"
	// +optional
	Type ControlPlaneMachineSetStrategyType `json:"type,omitempty"`

	// This is left as a struct to allow future rolling update
	// strategy configuration to be added later.
}

// ControlPlaneMachineSetStrategyType is an enumeration of different update strategies
// for the Control Plane Machines.
type ControlPlaneMachineSetStrategyType string

const (
	// RollingUpdate is the default update strategy type for a
	// ControlPlaneMachineSet. This will cause the ControlPlaneMachineSet to
	// first create a new Machine and wait for this to be Ready
	// before removing the Machine chosen for replacement.
	RollingUpdate ControlPlaneMachineSetStrategyType = "RollingUpdate"

	// Recreate causes the ControlPlaneMachineSet controller to first
	// remove a ControlPlaneMachine before creating its
	// replacement. This allows for scenarios with limited capacity
	// such as baremetal environments where additional capacity to
	// perform rolling updates is not available.
	Recreate ControlPlaneMachineSetStrategyType = "Recreate"

	// OnDelete causes the ControlPlaneMachineSet to only replace a
	// Machine once it has been marked for deletion. This strategy
	// makes the rollout of updated specifications into a manual
	// process. This allows users to test new configuration on
	// a single Machine without forcing the rollout of all of their
	// Control Plane Machines.
	OnDelete ControlPlaneMachineSetStrategyType = "OnDelete"
)

// FailureDomain represents the different configurations required to spread Machines
// across failure domains on different platforms.
// +union
type FailureDomains struct {
	// Platform identifies the platform for which the FailureDomain represents
	// +unionDiscriminator
	// +optional
	Platform configv1.PlatformType `json:"platform,omitempty"`

	// AWS configures failure domain information for the AWS platform
	// +optional
	AWS *[]AWSFailureDomain `json:"aws,omitempty"`

	// Azure configures failure domain information for the Azure platform
	// +optional
	Azure *[]AzureFailureDomain `json:"azure,omitempty"`

	// GCP configures failure domain information for the GCP platform
	// +optional
	GCP *[]GCPFailureDomain `json:"gcp,omitempty"`

	// OpenStack configures failure domain information for the OpenStack platform
	// +optional
	OpenStack *[]OpenStackFailureDomain `json:"openstack,omitempty"`
}

// AWSFailureDomain configures failure domain information for the AWS platform
type AWSFailureDomain struct {
	// Subnet is a reference to the subnet to use for this instance
	// +optional
	Subnet machinev1beta1.AWSResourceReference `json:"subnet,omitempty"`

	// Placement configures the placement information for this instance
	// +optional
	Placement AWSFailureDomainPlacement `json:"placement,omitempty"`
}

// AWSFailureDomainPlacement configures the placement information for the AWSFailureDomain
type AWSFailureDomainPlacement struct {
	// AvailabilityZone is the availability zone of the instance
	// +optional
	AvailabilityZone string `json:"availabilityZone,omitempty"`
}

// AzureFailureDomain configures failure domain information for the Azure platform
type AzureFailureDomain struct {
	// Availability Zone for the virtual machine.
	// If nil, the virtual machine should be deployed to no zone
	// +optional
	Zone *string `json:"zone,omitempty"`
}

// GCPFailureDomain configures failure domain information for the GCP platform
type GCPFailureDomain struct {
	// Zone is the zone in which the GCP machine provider will create the VM.
	// +optional
	Zone string `json:"zone"`
}

// OpenStackFailureDomain configures failure domain information for the OpenStack platform
type OpenStackFailureDomain struct {
	// The availability zone from which to launch the server.
	// +optional
	AvailabilityZone string `json:"availabilityZone,omitempty"`
}

// ControlPlaneMachineSetStatus represents the status of the ControlPlaneMachineSet CRD.
type ControlPlaneMachineSetStatus struct {
	// ObservedGeneration is the most recent generation observed for this
	// ControlPlaneMachineSet. It corresponds to the ControlPlaneMachineSets's generation,
	// which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Replicas is the number of Control Plane Machines created by the
	// ControlPlaneMachineSet controller.
	// Note that during update operations this value may differ from the
	// desired replica count.
	Replicas int32 `json:"replicas"`

	// ReadyReplicas is the number of Control Plane Machines created by the
	// ControlPlaneMachineSet controller which are ready.
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// UpdatedReplicas is the number of non-terminated Control Plane Machines
	// created by the ControlPlaneMachineSet controller that have the desired
	// provider spec.
	UpdatedReplicas int32 `json:"updatedReplicas,omitempty"`

	// UnavailableReplicas is the number of Control Plane Machines that are
	// still required before the ControlPlaneMachineSet reaches the desired
	// available capacity. When this value is non-zero, the number of
	// ReadyReplicas is less than the desired Replicas.
	UnavailableReplicas int32 `json:"unavailableReplicas,omitempty"`

	// Conditions represents the observations of the ControlPlaneMachineSet's current state.
	// Known .status.conditions.type are: (TODO)
	// TODO: Identify different condition types/reasons that will be needed.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ControlPlaneMachineSetList contains a list of ControlPlaneMachineSet
// Compatibility level 2: Stable within a major release for a minimum of 9 months or 3 minor releases (whichever is longer).
// +openshift:compatibility-gen:level=2
type ControlPlaneMachineSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ControlPlaneMachineSet `json:"items"`
}
