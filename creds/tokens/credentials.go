package tokens

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neofs-s3-gw/api/cache"
	"github.com/nspcc-dev/neofs-s3-gw/creds/accessbox"
	cid "github.com/nspcc-dev/neofs-sdk-go/container/id"
	"github.com/nspcc-dev/neofs-sdk-go/object/address"
	oid "github.com/nspcc-dev/neofs-sdk-go/object/id"
	"github.com/nspcc-dev/neofs-sdk-go/owner"
)

type (
	// Credentials is a bearer token get/put interface.
	Credentials interface {
		GetBox(context.Context, *address.Address) (*accessbox.Box, error)
		Put(context.Context, *cid.ID, *owner.ID, *accessbox.AccessBox, uint64, ...*keys.PublicKey) (*address.Address, error)
	}

	cred struct {
		key   *keys.PrivateKey
		neoFS NeoFS
		cache *cache.AccessBoxCache
	}
)

// PrmObjectCreate groups parameters of objects created by credential tool.
type PrmObjectCreate struct {
	// NeoFS identifier of the object creator.
	Creator owner.ID

	// NeoFS container to store the object.
	Container cid.ID

	// File name.
	Filename string

	// Last NeoFS epoch of the object lifetime.
	ExpirationEpoch uint64

	// Object payload.
	Payload []byte
}

// NeoFS represents virtual connection to NeoFS network.
type NeoFS interface {
	// CreateObject creates and saves a parameterized object in the specified
	// NeoFS container from a specific user. It sets 'Timestamp' attribute to the current time.
	// It returns the ID of the saved object.
	//
	// It returns exactly one non-nil value. It returns any error encountered which
	// prevented the object from being created.
	CreateObject(context.Context, PrmObjectCreate) (*oid.ID, error)

	// ReadObjectPayload reads payload of the object from NeoFS network by address
	// into memory.
	//
	// It returns exactly one non-nil value. It returns any error encountered which
	// prevented the object payload from being read.
	ReadObjectPayload(context.Context, address.Address) ([]byte, error)
}

var (
	// ErrEmptyPublicKeys is returned when no HCS keys are provided.
	ErrEmptyPublicKeys = errors.New("HCS public keys could not be empty")
	// ErrEmptyBearerToken is returned when no bearer token is provided.
	ErrEmptyBearerToken = errors.New("Bearer token could not be empty")
)

var _ = New

// New creates a new Credentials instance using the given cli and key.
func New(neoFS NeoFS, key *keys.PrivateKey, config *cache.Config) Credentials {
	return &cred{neoFS: neoFS, key: key, cache: cache.NewAccessBoxCache(config)}
}

func (c *cred) GetBox(ctx context.Context, addr *address.Address) (*accessbox.Box, error) {
	cachedBox := c.cache.Get(addr)
	if cachedBox != nil {
		return cachedBox, nil
	}

	box, err := c.getAccessBox(ctx, addr)
	if err != nil {
		return nil, err
	}

	cachedBox, err = box.GetBox(c.key)
	if err != nil {
		return nil, err
	}

	if err = c.cache.Put(addr, cachedBox); err != nil {
		return nil, err
	}

	return cachedBox, nil
}

func (c *cred) getAccessBox(ctx context.Context, addr *address.Address) (*accessbox.AccessBox, error) {
	data, err := c.neoFS.ReadObjectPayload(ctx, *addr)
	if err != nil {
		return nil, fmt.Errorf("read payload: %w", err)
	}

	// decode access box
	var box accessbox.AccessBox
	if err = box.Unmarshal(data); err != nil {
		return nil, err
	}

	return &box, nil
}

func (c *cred) Put(ctx context.Context, idCnr *cid.ID, issuer *owner.ID, box *accessbox.AccessBox, expiration uint64, keys ...*keys.PublicKey) (*address.Address, error) {
	if len(keys) == 0 {
		return nil, ErrEmptyPublicKeys
	} else if box == nil {
		return nil, ErrEmptyBearerToken
	}
	data, err := box.Marshal()
	if err != nil {
		return nil, err
	}

	idObj, err := c.neoFS.CreateObject(ctx, PrmObjectCreate{
		Creator:         *issuer,
		Container:       *idCnr,
		Filename:        strconv.FormatInt(time.Now().Unix(), 10) + "_access.box",
		ExpirationEpoch: expiration,
		Payload:         data,
	})
	if err != nil {
		return nil, err
	}

	addr := address.NewAddress()
	addr.SetObjectID(idObj)
	addr.SetContainerID(idCnr)
	return addr, nil
}
