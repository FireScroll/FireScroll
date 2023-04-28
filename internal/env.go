package internal

import (
	"github.com/danthegoodman1/Firescroll/utils"
)

var (
	Env_InternalPort = utils.EnvOrDefault("INTERNAL_PORT", "8091")
)
