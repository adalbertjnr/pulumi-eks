package types

import "errors"

var ErrNotErrorServiceSkipped = errors.New("service skipped")
var ErrNotErrorDisabledOIDCProvider = errors.New("disabled oidc provider")
