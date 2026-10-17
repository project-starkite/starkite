package k8s

import (
	"github.com/project-starkite/starkite/libkite"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// AttrDict is an alias to the canonical libkite.AttrDict.
type AttrDict = libkite.AttrDict

// NewAttrDict constructs an AttrDict using libkite.NewAttrDict.
var NewAttrDict = libkite.NewAttrDict

func unstructuredToAttrDict(obj *unstructured.Unstructured) *AttrDict {
	if obj == nil {
		return nil
	}
	return NewAttrDict(obj.Object)
}
