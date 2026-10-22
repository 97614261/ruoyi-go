# Invoke as a child script with an inherited exit value to reproduce automatic
# variable shadowing. Test doubles only; never runs Go/Node or the database.
param([string]$Runner, [string]$Tool, [string]$Formatter)
$LASTEXITCODE = 73
& $Runner -GoPath $Tool -GofmtPath $Formatter -NodePath $Tool
exit $LASTEXITCODE
