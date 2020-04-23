package volume_mount_options

import (
	"strconv"

	"code.cloudfoundry.org/volume-mount-options/utils"
	"github.com/pkg/errors"
)

type MountOptsMask struct {
	// set of options that are allowed to be provided by the user
	Allowed []string

	// set of default values that will be used if not otherwise provided
	Defaults map[string]interface{}

	// set of key permutations
	KeyPerms map[string]string

	// set of options that, if provided,  will be silently ignored
	Ignored []string

	// set of options that must be provided
	Mandatory []string

	SloppyMount bool

	ValidationFunc ValidationFuncI
}
//go:generate counterfeiter . ValidationFuncI
type ValidationFuncI interface {
	Validate(string, string) error
}

type ValidationFunc func(string, string) error

func (v ValidationFunc) Validate(a string, b string) error {
	return v(a, b)
}

func NewMountOptsMask(allowed []string, defaults map[string]interface{}, keyPerms map[string]string, ignored, mandatory []string, f ValidationFuncI) (MountOptsMask, error) {
	mask := MountOptsMask{
		Allowed:   allowed,
		Defaults:  defaults,
		KeyPerms:  keyPerms,
		Ignored:   ignored,
		Mandatory: mandatory,
		ValidationFunc: f,
	}

	if defaults == nil {
		mask.Defaults = make(map[string]interface{})
	}

	if v, ok := defaults["sloppy_mount"]; ok {
		vc := utils.InterfaceToString(v)

		var err error
		mask.SloppyMount, err = strconv.ParseBool(vc)

		if err != nil {
			return MountOptsMask{}, errors.Wrap(err, "Invalid sloppy_mount option")
		}
	}

	return mask, nil
}
