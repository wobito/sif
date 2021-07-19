// Copyright (c) 2021, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package sif

import (
	"fmt"
	"io"
	"os"
	"time"
)

// descriptorOpts accumulates data object options.
type descriptorOpts struct {
	groupID   uint32
	linkID    uint32
	alignment int
	name      string
	extra     interface{}
	t         time.Time
}

// DescriptorInputOpt are used to specify data object options.
type DescriptorInputOpt func(Datatype, *descriptorOpts) error

// OptGroupID specifies groupID as data object group ID.
func OptGroupID(groupID uint32) DescriptorInputOpt {
	return func(_ Datatype, opts *descriptorOpts) error {
		if groupID == 0 {
			return ErrInvalidGroupID
		}
		opts.groupID = groupID
		return nil
	}
}

// OptLinkedID specifies that the data object is linked to the data object group with the specified
// ID.
func OptLinkedID(id uint32) DescriptorInputOpt {
	return func(_ Datatype, opts *descriptorOpts) error {
		if id == 0 {
			return ErrInvalidObjectID
		}
		opts.linkID = id
		return nil
	}
}

// OptLinkedGroupID specifies that the data object is linked to the data object group with the
// specified groupID.
func OptLinkedGroupID(groupID uint32) DescriptorInputOpt {
	return func(_ Datatype, opts *descriptorOpts) error {
		if groupID == 0 {
			return ErrInvalidGroupID
		}
		opts.linkID = groupID | DescrGroupMask
		return nil
	}
}

// OptObjectAlignment specifies n as the data alignment requirement.
func OptObjectAlignment(n int) DescriptorInputOpt {
	return func(_ Datatype, opts *descriptorOpts) error {
		opts.alignment = n
		return nil
	}
}

// OptObjectName specifies name as the data object name.
func OptObjectName(name string) DescriptorInputOpt {
	return func(_ Datatype, opts *descriptorOpts) error {
		opts.name = name
		return nil
	}
}

// OptObjectTime specifies t as the dat object creation time.
func OptObjectTime(t time.Time) DescriptorInputOpt {
	return func(_ Datatype, opts *descriptorOpts) error {
		opts.t = t
		return nil
	}
}

type unexpectedDataTypeError struct {
	got  Datatype
	want Datatype
}

func (e *unexpectedDataTypeError) Error() string {
	return fmt.Sprintf("unexpected data type %v, expected %v", e.got, e.want)
}

func (e *unexpectedDataTypeError) Is(target error) bool {
	t, ok := target.(*unexpectedDataTypeError)
	if !ok {
		return false
	}
	return (e.got == t.got || t.got == 0) &&
		(e.want == t.want || t.want == 0)
}

// OptCryptoMessageMetadata sets metadata for a crypto message data object. The format type is set
// to ft, and the message type is set to mt.
//
// If this option is applied to a data object with an incompatible type, an error is returned.
func OptCryptoMessageMetadata(ft Formattype, mt Messagetype) DescriptorInputOpt {
	return func(t Datatype, opts *descriptorOpts) error {
		if got, want := t, DataCryptoMessage; got != want {
			return &unexpectedDataTypeError{got, want}
		}

		m := cryptoMessage{
			Formattype:  ft,
			Messagetype: mt,
		}

		opts.extra = m
		return nil
	}
}

// OptPartitionMetadata sets metadata for a partition data object. The filesystem type is set to
// fs, the partition type is set to pt, and the CPU architecture is set to arch. The value of arch
// should be the architecture as represented by the Go runtime.
//
// If this option is applied to a data object with an incompatible type, an error is returned.
func OptPartitionMetadata(fs Fstype, pt Parttype, arch string) DescriptorInputOpt {
	return func(t Datatype, opts *descriptorOpts) error {
		if got, want := t, DataPartition; got != want {
			return &unexpectedDataTypeError{got, want}
		}

		sifarch := GetSIFArch(arch)
		if sifarch == HdrArchUnknown {
			return fmt.Errorf("unknown architecture: %v", arch)
		}

		p := partition{
			Fstype:   fs,
			Parttype: pt,
		}
		copy(p.Arch[:], sifarch)

		opts.extra = p
		return nil
	}
}

// OptSignatureMetadata sets metadata for a signature data object. The hash type is set to t, and
// the signing entity fingerprint is set to fp.
//
// If this option is applied to a data object with an incompatible type, an error is returned.
func OptSignatureMetadata(ht Hashtype, fp [20]byte) DescriptorInputOpt {
	return func(t Datatype, opts *descriptorOpts) error {
		if got, want := t, DataSignature; got != want {
			return &unexpectedDataTypeError{got, want}
		}

		s := signature{
			Hashtype: ht,
		}
		copy(s.Entity[:], fp[:])

		opts.extra = s
		return nil
	}
}

// DescriptorInput describes a new data object.
type DescriptorInput struct {
	Datatype Datatype
	Groupid  uint32
	Link     uint32
	Size     int64
	Fname    string

	alignment int
	Fp        io.Reader

	opts descriptorOpts
}

// NewDescriptorInput returns a DescriptorInput representing a data object of type t, with contents
// read from r, configured according to opts.
//
// It is possible (and often necessary) to store additional metadata related to certain types of
// data objects. Consider supplying options such as OptCryptoMessageMetadata, OptPartitionMetadata,
// and OptSignatureMetadata for this purpose.
//
// By default, the data object will not be part of a data object group. To override this behavior,
// use OptGroupID. To link this data object, use OptLinkedID or OptLinkedGroupID.
//
// By default, the data object will be aligned according to the system's memory page size. To
// override this behavior, consider using OptObjectAlignment.
//
// By default, no name is set for data object. To set a name, use OptObjectName.
func NewDescriptorInput(t Datatype, r io.Reader, opts ...DescriptorInputOpt) (DescriptorInput, error) {
	dopts := descriptorOpts{
		alignment: os.Getpagesize(),
		t:         time.Now(),
	}

	for _, opt := range opts {
		if err := opt(t, &dopts); err != nil {
			return DescriptorInput{}, fmt.Errorf("%w", err)
		}
	}

	di := DescriptorInput{
		Datatype:  t,
		Fp:        r,
		Groupid:   dopts.groupID | DescrGroupMask,
		Link:      dopts.linkID,
		alignment: dopts.alignment,
		Fname:     dopts.name,
		opts:      dopts,
	}

	return di, nil
}

// fillDescriptor fills d according to di.
func (di DescriptorInput) fillDescriptor(d *Descriptor) error {
	d.Datatype = di.Datatype
	d.Groupid = di.opts.groupID | DescrGroupMask
	d.Link = di.opts.linkID
	d.Ctime = di.opts.t.UTC().Unix()
	d.Mtime = di.opts.t.UTC().Unix()
	d.UID = 0
	d.GID = 0

	if err := d.setName(di.opts.name); err != nil {
		return err
	}

	return d.setExtra(di.opts.extra)
}
