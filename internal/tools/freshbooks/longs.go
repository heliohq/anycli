package freshbooks

// The `Long` text for every accounting leaf lives here rather than in
// resources.go, because the six resource families are built by one shared set
// of verb constructors: the prose that separates `invoice create` from
// `expense create` has to travel on the resource spec, and 23 inline literals
// would bury the builders they sit in.

const longRoot = "Every accounting URL is scoped to an `account_id`, which is NOT the login\n" +
	"identity — it lives in `business_memberships[].business.account_id`. Commands\n" +
	"resolve it from the connected identity: exactly one accounting account is\n" +
	"used silently, several fail with the available ids so `--account <accountId>`\n" +
	"can pick one, and none fails outright. Nothing is guessed.\n" +
	"\n" +
	"Passing `--account` skips that identity lookup, so an accounting command\n" +
	"without it costs one extra request. Read the mapping once with `me`, then\n" +
	"keep passing the id.\n" +
	"\n" +
	"FreshBooks' `{\"response\":{\"result\":{…}}}` nesting is unwrapped before output.\n" +
	"`list` emits `{\"items\":[…],\"page\":…,\"pages\":…,\"per_page\":…,\"total\":…}`;\n" +
	"every other verb emits the resource object on its own.\n" +
	"\n" +
	"Money is an object, never a bare number: `{\"amount\":\"100.00\",\"code\":\"USD\"}`.\n" +
	"\n" +
	"Writes take the resource fields as a plain JSON object through `--data` or\n" +
	"`--file`; the `{\"invoice\":{…}}` / `{\"client\":{…}}` wrapper FreshBooks expects\n" +
	"is added for you, so do not send it.\n" +
	"\n" +
	"Coverage is the accounting surface only — clients, invoices, expenses,\n" +
	"estimates, payments and the billable-item catalog. Time tracking and projects\n" +
	"are keyed by business id rather than account id and have no commands here."

const longMe = "Returns the identity FreshBooks holds for the connected token together with\n" +
	"its `business_memberships`, each carrying a `business.account_id`. Those\n" +
	"account ids are what every accounting command needs and this is the only\n" +
	"command that reads them. It touches no accounting data, so it stays\n" +
	"available even on an identity that has no business attached."

const (
	longClientList = "Clients are the bill-to party an invoice's `customerid` points at, so this is\n" +
		"the usual first call when only a company or email address is known. `--query`\n" +
		"is repeatable and passes FreshBooks search filters through verbatim, e.g.\n" +
		"`--query 'search[email]=a@b.com'`. Paging is `--page` (1-based) and\n" +
		"`--per-page`, both omitted from the request when unset so FreshBooks' own\n" +
		"defaults apply; `pages` and `total` in the envelope say how far the set goes."

	longClientGet = "Takes the client id that `client list` returns and emits the client record on\n" +
		"its own. There is no `client delete` in this tool — an obsolete client can be\n" +
		"edited with `client update` but not removed, so a stale record stays in\n" +
		"`client list` forever."

	longClientCreate = "`--data` (or `--file`) is a JSON object of plain client fields —\n" +
		"`organization`, `email`, `fname`, `lname` and the rest — with no\n" +
		"`{\"client\":…}` wrapper. Exactly one of the two flags is required and they are\n" +
		"mutually exclusive. The `id` on the returned record is what an invoice's\n" +
		"`customerid` refers to, so the client has to exist before the invoice does."

	longClientUpdate = "A partial PUT: only the fields present in `--data`/`--file` change, the rest\n" +
		"keep their current values, and the whole updated client comes back. Same\n" +
		"unwrapped JSON object as `client create` — no `{\"client\":…}` wrapper. Because\n" +
		"there is no delete verb for clients, this is also how a client is retired."
)

const (
	longInvoiceList = "`--query` is repeatable and carries FreshBooks' search filters verbatim; the\n" +
		"two that matter here are `search[customerid]=<clientId>` for one client's\n" +
		"invoices and `search[updated_min]=<date>` for a window. Paging is `--page`\n" +
		"(1-based) and `--per-page`, with `pages` and `total` in the envelope. Each\n" +
		"row carries `vis_state`, which `invoice delete` sets to 1."

	longInvoiceGet = "Returns the whole invoice — its `lines`, its totals as money objects and its\n" +
		"`vis_state`, where 1 means `invoice delete` soft-deleted it. When only the\n" +
		"client is known, `invoice list --query 'search[customerid]=<clientId>'` is\n" +
		"the way to reach the id."

	longInvoiceCreate = "`--data`/`--file` is a plain JSON object of invoice fields with no\n" +
		"`{\"invoice\":…}` wrapper: `customerid` (an existing client's id, from `client\n" +
		"list` or `client create`), `create_date`, and `lines`, where each line's\n" +
		"`unit_cost` is a money object such as `{\"amount\":\"100.00\",\"code\":\"USD\"}` and\n" +
		"never a bare number. Creating sends nothing to the client — the invoice sits\n" +
		"unsent until `invoice send`."

	longInvoiceUpdate = "A partial PUT: only the fields in `--data`/`--file` change and the updated\n" +
		"invoice comes back whole. Replacing `lines` replaces the entire array, so\n" +
		"send every line that should survive, each with its money-object `unit_cost`.\n" +
		"Editing an invoice after `invoice send` does not re-send it."

	longInvoiceDelete = "A soft delete: it PUTs `vis_state: 1` instead of removing anything, and emits\n" +
		"the updated invoice rather than a deletion receipt, so numbering and history\n" +
		"survive. Invoices are the only family here with a delete verb — clients,\n" +
		"expenses, estimates and payments have none."

	longInvoiceSend = "Sets `action_email` on the invoice, which both emails it and flips it to sent\n" +
		"— there is no separate mark-as-sent. It goes to the client's stored email\n" +
		"address unless `--to` is passed; `--to` is repeatable and replaces the\n" +
		"default recipients. FreshBooks rejects the call when the client has no email\n" +
		"and no `--to` was given. The mail leaves immediately and cannot be recalled."
)

const (
	longExpenseList = "`--query` is repeatable and passes FreshBooks expense filters through\n" +
		"verbatim, e.g. `search[date_min]=2026-01-01` for a window or\n" +
		"`search[clientid]=<id>` for one client's billable expenses. Paging is\n" +
		"`--page` (1-based) and `--per-page`; `total` in the envelope is the count of\n" +
		"matching expenses, not a sum of money."

	longExpenseGet = "Emits the expense record on its own, with `amount` as a money object\n" +
		"(`{\"amount\":\"12.50\",\"code\":\"USD\"}`) rather than a number. Attached receipt\n" +
		"images are not downloadable through this tool."

	longExpenseCreate = "`--data`/`--file` is a plain JSON object of expense fields with no\n" +
		"`{\"expense\":…}` wrapper: `amount` as a money object, `date`, a `categoryid`\n" +
		"from the account's expense categories, and optionally `clientid` plus\n" +
		"`billable` to bill it on. Expenses cannot be deleted through this tool, so a\n" +
		"mistaken one has to be corrected with `expense update`."

	longExpenseUpdate = "A partial PUT: only the fields in `--data`/`--file` change and the whole\n" +
		"updated expense comes back. This is the only correction path for a bad\n" +
		"expense, since the family has no delete verb."
)

const (
	longEstimateList = "Paging is `--page` (1-based) and `--per-page`, plus repeatable `--query`\n" +
		"filters such as `search[customerid]=<clientId>`. An estimate is a quote, not\n" +
		"an invoice: accepting one does not create the invoice, which still has to be\n" +
		"raised with `invoice create`."

	longEstimateGet = "Emits the estimate with its `lines` and money-object totals. Estimates are\n" +
		"create-and-read only here — there is no update or delete verb, so a wrong\n" +
		"estimate has to be superseded by a new one or fixed in FreshBooks itself."

	longEstimateCreate = "`--data`/`--file` is a plain JSON object of estimate fields with no\n" +
		"`{\"estimate\":…}` wrapper, shaped like an invoice: `customerid`, `create_date`\n" +
		"and `lines` whose `unit_cost` is a money object. It is not reachable again\n" +
		"for editing — this family exposes no update or delete — and it is not\n" +
		"emailed; there is no estimate send verb either."
)

const (
	longPaymentList = "Payments are records of money already received against invoices, so this is\n" +
		"the read for reconciliation rather than for collecting. Paging is `--page`\n" +
		"(1-based) and `--per-page`, with repeatable `--query` filters such as\n" +
		"`search[invoiceid]=<id>`. Amounts come back as money objects."

	longPaymentGet = "Emits one payment record, its `amount` a money object and its `invoiceid`\n" +
		"naming the invoice it settles. Payments cannot be edited or reversed through\n" +
		"this tool — the family exposes only `list`, `get` and `create`."

	longPaymentCreate = "Records a payment already received; it moves no money and charges nothing.\n" +
		"`--data`/`--file` is a plain JSON object with no `{\"payment\":…}` wrapper:\n" +
		"`invoiceid` for the invoice being settled, `amount` as a money object, `date`\n" +
		"and a payment `type`. It cannot be undone here — there is no payment update\n" +
		"or delete verb — so check the invoice with `invoice get` first."
)

const (
	longItemList = "The billable-item catalog: reusable names, descriptions and unit costs to\n" +
		"copy into an invoice's `lines`. Read-only — items are created and edited in\n" +
		"FreshBooks itself, and nothing here writes to the catalog. Paging is\n" +
		"`--page` (1-based) and `--per-page` with repeatable `--query` filters."

	longItemGet = "Emits one catalog item, including its `unit_cost` as a money object. Nothing\n" +
		"links the item to an invoice automatically: an invoice line is a copy of\n" +
		"these values at the time it is written, so later catalog edits do not reach\n" +
		"invoices already raised."
)
