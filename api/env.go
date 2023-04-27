package api

import (
	"github.com/danthegoodman1/FanoutDB/utils"
)

var (
	Env_APIPort = utils.EnvOrDefault("API_PORT", "8070")
)
