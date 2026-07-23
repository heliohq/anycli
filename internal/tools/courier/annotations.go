package courier

// readOnly / writeAction carry the design-318 side-effect annotation applied to
// every runnable leaf command in this package's cobra tree.
var readOnly = map[string]string{"anycli.side_effect": "false"}
var writeAction = map[string]string{"anycli.side_effect": "true"}
