package ramp

// The `Long` text for the leaves built by newListCmd / newGetByIDCmd lives
// here: one pair of constructors builds ten of the fifteen leaves, so the
// prose that separates a reimbursement from a location has to be passed in
// rather than written inside the builder.

const (
	longTransactionList = "Card spend only — money employees paid out of pocket lives in\n" +
		"`reimbursement list`, and the two together are the whole picture. The only\n" +
		"flags are the cursor ones, so there is no server-side narrowing by state,\n" +
		"date, card or user here; those go through the passthrough, e.g.\n" +
		"`get /developer/v1/transactions --param state=CLEARED`, which reaches the\n" +
		"same endpoint with Ramp's own query parameters."

	longTransactionGet = "Takes the transaction id from a list page; there is no lookup by merchant,\n" +
		"amount or date, so the id has to be found first. The record names its\n" +
		"cardholder, card, department and location as ids, which `user get`,\n" +
		"`card virtual` / `card physical`, `department get` and `location get`\n" +
		"resolve into names."

	longReimbursementList = "Out-of-pocket expenses employees claimed back. These never appear in\n" +
		"`transaction list`, which covers card spend only, so a \"what did we spend\"\n" +
		"answer that reads just one of the two is incomplete. Cursor flags only —\n" +
		"any state or date narrowing goes through\n" +
		"`get /developer/v1/reimbursements --param …`."

	longReimbursementGet = "Takes the reimbursement id from a list page. Returns the claim with its\n" +
		"amount, state and the user who filed it, referenced by user id. Nothing\n" +
		"here approves, rejects or pays a reimbursement — the state can be read but\n" +
		"not changed."

	longUserList = "Every person on the business, cardholder or not, which makes this the\n" +
		"id-to-name resolution for transactions and reimbursements: those records\n" +
		"carry only a user id. Roles and department membership come back on the same\n" +
		"rows, so a \"who spends in which department\" mapping needs no second call."

	longUserGet = "Takes the Ramp user id that a transaction, card or reimbursement referenced.\n" +
		"There is no lookup by email or name, so resolving a person from their\n" +
		"address means paging `user list` and matching locally."

	longDepartmentList = "Departments are one of the two dimensions Ramp groups spend by, the other\n" +
		"being locations. The ids here are what transactions and users carry, so\n" +
		"this is the lookup that turns a department id into a name. Read-only, like\n" +
		"everything else: departments are managed in Ramp itself."

	longDepartmentGet = "Takes a department id as it appears on a transaction or user record. Returns\n" +
		"the department itself — not its members or its spend, neither of which has\n" +
		"a command here; approach both from `user list` and `transaction list`."

	longLocationList = "Locations are the second grouping dimension alongside departments, and the\n" +
		"ids here are the ones transactions and users reference. Read-only —\n" +
		"locations are created and edited in Ramp itself."

	longLocationGet = "Takes a location id as it appears on a transaction or user record and\n" +
		"returns the location alone. Its transactions are not included; filter them\n" +
		"from `transaction list` or through the `get` passthrough instead."
)
