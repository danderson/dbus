package dbus

import (
	"path"
	"strings"
)

// An ObjectPath is a filesystem-like path for an [Object] exposed on
// over DBus.
type ObjectPath string

// Clean returns the result of applying [path.Clean] to the object
// path.
func (p ObjectPath) Clean() ObjectPath {
	return ObjectPath(path.Clean(string(p)))
}

func (p ObjectPath) String() string {
	return string(p.Clean())
}

// Valid reports whether p is a valid object path.
func (p ObjectPath) Valid() bool {
	return path.IsAbs(string(p.Clean()))
}

// Child returns the object path at the given relative path from the
// current object.
func (p ObjectPath) Child(relPath string) ObjectPath {
	return ObjectPath(path.Join(string(p.Clean()), relPath))
}

// IsChildOf reports whether p is a child of the given parent.
func (p ObjectPath) IsChildOf(parent ObjectPath) bool {
	sparent := string(parent.Clean())
	sp := string(p.Clean())
	if len(sp) <= len(sparent) {
		return false
	}
	return strings.HasPrefix(sp, sparent+"/")
}
