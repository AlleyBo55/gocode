package data

import _ "embed"

//go:embed commands.json
var CommandsJSON []byte

//go:embed tools.json
var ToolsJSON []byte
