package verify

import (
	"testing"
	"time"

	"github.com/flynn/go-tuf/data"
	"github.com/flynn/go-tuf/sign"
	"golang.org/x/crypto/ed25519"

	. "gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

type VerifySuite struct{}

var _ = Suite(&VerifySuite{})

func (VerifySuite) Test(c *C) {
	type test struct {
		name  string
		keys  []*data.Key
		roles map[string]*data.Role
		s     *data.Signed
		ver   int
		exp   *time.Time
		typ   string
		role  string
		err   error
		mut   func(*test)
	}

	expiredTime := time.Now().Add(-time.Hour)
	minVer := 10
	tests := []test{
		{
			name: "no signatures",
			mut:  func(t *test) { t.s.Signatures = []data.Signature{} },
			err:  ErrNoSignatures,
		},
		{
			name: "unknown role",
			role: "foo",
			err:  ErrUnknownRole,
		},
		{
			name: "wrong signature method",
			mut:  func(t *test) { t.s.Signatures[0].Method = "foo" },
			err:  ErrWrongMethod,
		},
		{
			name: "signature wrong length",
			mut:  func(t *test) { t.s.Signatures[0].Signature = []byte{0} },
			err:  ErrInvalid,
		},
		{
			name: "key missing from role",
			mut:  func(t *test) { t.roles["root"].KeyIDs = nil },
			err:  ErrRoleThreshold,
		},
		{
			name: "invalid signature",
			mut:  func(t *test) { t.s.Signatures[0].Signature = make([]byte, ed25519.SignatureSize) },
			err:  ErrInvalid,
		},
		{
			name: "not enough signatures",
			mut:  func(t *test) { t.roles["root"].Threshold = 2 },
			err:  ErrRoleThreshold,
		},
		{
			name: "exactly enough signatures",
		},
		{
			name: "more than enough signatures",
			mut: func(t *test) {
				k, _ := sign.GenerateEd25519Key()
				sign.Sign(t.s, k.Signer())
				t.keys = append(t.keys, k.PublicData())
				t.roles["root"].KeyIDs = append(t.roles["root"].KeyIDs, k.PublicData().ID())
			},
		},
		{
			name: "duplicate key id",
			mut: func(t *test) {
				t.roles["root"].Threshold = 2
				t.s.Signatures = append(t.s.Signatures, t.s.Signatures[0])
			},
			err: ErrRoleThreshold,
		},
		{
			name: "unknown key",
			mut: func(t *test) {
				k, _ := sign.GenerateEd25519Key()
				sign.Sign(t.s, k.Signer())
			},
		},
		{
			name: "unknown key below threshold",
			mut: func(t *test) {
				k, _ := sign.GenerateEd25519Key()
				sign.Sign(t.s, k.Signer())
				t.roles["root"].Threshold = 2
			},
			err: ErrRoleThreshold,
		},
		{
			name: "unknown keys in db",
			mut: func(t *test) {
				k, _ := sign.GenerateEd25519Key()
				sign.Sign(t.s, k.Signer())
				t.keys = append(t.keys, k.PublicData())
			},
		},
		{
			name: "unknown keys in db below threshold",
			mut: func(t *test) {
				k, _ := sign.GenerateEd25519Key()
				sign.Sign(t.s, k.Signer())
				t.keys = append(t.keys, k.PublicData())
				t.roles["root"].Threshold = 2
			},
			err: ErrRoleThreshold,
		},
		{
			name: "wrong type",
			typ:  "bar",
			err:  ErrWrongMetaType,
		},
		{
			name: "low version",
			ver:  minVer - 1,
			err:  ErrLowVersion{minVer - 1, minVer},
		},
		{
			name: "expired",
			exp:  &expiredTime,
			err:  ErrExpired{expiredTime},
		},
	}
	for _, t := range tests {
		if t.role == "" {
			t.role = "root"
		}
		if t.ver == 0 {
			t.ver = minVer
		}
		if t.exp == nil {
			expires := time.Now().Add(time.Hour)
			t.exp = &expires
		}
		if t.typ == "" {
			t.typ = t.role
		}
		if t.keys == nil && t.s == nil {
			k, _ := sign.GenerateEd25519Key()
			t.s, _ = sign.Marshal(&signedMeta{Type: t.typ, Version: t.ver, Expires: *t.exp}, k.Signer())
			t.keys = []*data.Key{k.PublicData()}
		}
		if t.roles == nil {
			t.roles = map[string]*data.Role{
				"root": &data.Role{
					KeyIDs:    []string{t.keys[0].ID()},
					Threshold: 1,
				},
			}
		}
		if t.mut != nil {
			t.mut(&t)
		}

		db := NewDB()
		for _, k := range t.keys {
			err := db.AddKey(k.ID(), k)
			c.Assert(err, IsNil)
		}
		for n, r := range t.roles {
			err := db.AddRole(n, r)
			c.Assert(err, IsNil)
		}

		err := db.Verify(t.s, t.role, minVer)
		if e, ok := t.err.(ErrExpired); ok {
			assertErrExpired(c, err, e)
		} else {
			c.Assert(err, DeepEquals, t.err, Commentf("name = %s", t.name))
		}
	}
}

func assertErrExpired(c *C, err error, expected ErrExpired) {
	actual, ok := err.(ErrExpired)
	if !ok {
		c.Fatalf("expected err to have type ErrExpired, got %T", err)
	}
	c.Assert(actual.Expired.Unix(), Equals, expected.Expired.Unix())
}
