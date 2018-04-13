package tinyop

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	ResourceKind   = "EchoServer"
	ResourcePlural = "echoservers"
	GroupName      = "tinyop.objectrocket.com"
	ShortName      = "echoserver"
	Version        = "v1alpha1"
)

var (
	Name               = fmt.Sprintf("%s.%s", ResourcePlural, GroupName)
	SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: Version}
)
