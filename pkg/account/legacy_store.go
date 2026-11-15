package account

import (
	"errors"
	"github.com/zalando/go-keyring"
)

// ImportLegacySession is used only during migration of an existing workspace.
// Fresh installations must never adopt another folder's global member session.
func ImportLegacySession(base, group string, destination Store) error {
	token, err := keyring.Get("Takealot-GoCMS", credentialKey(base, group))
	if err != nil {
		return err
	}
	if !validBearerToken(token) {
		return errors.New("旧会员凭据无效，请重新登录")
	}
	return destination.Save(token)
}
