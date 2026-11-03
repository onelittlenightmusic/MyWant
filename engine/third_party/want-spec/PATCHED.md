# want-spec (vendored, ahead of release)

A copy of github.com/onelittlenightmusic/want-spec v0.1.27 with the robot's
resources added (issue #53), used through `replace` in engine/go.mod and
client/go.mod until want-spec releases them.

Added (copy these into want-spec as they are, then release a patch):

- `types_robot.go` — ResourceMeta, RobotStep, RobotCard, ResourceAPIVersion
- `types_plan.go`  — Plan, PlanSpec, PlanList, PlanAction*, Validate/Normalize
- `types_act.go`   — Act, ActSpec, ActStatus, ActList, Act* phases, Finished/Validate
- `spec/plan-spec.yaml`, `spec/act-spec.yaml`

Once released: drop the `replace github.com/onelittlenightmusic/want-spec`
lines, bump the require to the new version, and delete this directory.
