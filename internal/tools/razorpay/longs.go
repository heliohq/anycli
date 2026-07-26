package razorpay

// The `Long` text for the fourteen resource leaves lives here because both
// leaves of every family are built by one shared pair of constructors: the
// prose that separates a settlement from a subscription has to travel on the
// resource spec rather than sit inline in the builder.

const longRoot = "This tool is READ-ONLY over the Razorpay gateway. There is no verb that\n" +
	"captures a payment, issues a refund, creates an order or sends a payment\n" +
	"link, and RazorpayX banking (payouts, contacts, fund accounts) is absent\n" +
	"entirely. A request to move money cannot be satisfied here — say so rather\n" +
	"than reaching for a flag that does not exist.\n" +
	"\n" +
	"Amounts are integers in the SMALLEST currency unit — paise for INR, so\n" +
	"₹100 arrives as `amount: 10000`. Nothing is rescaled on the way out; divide\n" +
	"before reporting a figure to a human.\n" +
	"\n" +
	"Every resource exposes the same two verbs, `list` and `get <id>`, and the\n" +
	"provider's JSON is printed verbatim — including the\n" +
	"`{\"entity\":\"collection\",\"count\":N,\"items\":[…]}` envelope on lists.\n" +
	"\n" +
	"Every `list` takes `--count` (Razorpay caps it at 100), `--skip` as the\n" +
	"offset, and `--from` / `--to` as inclusive Unix-SECOND bounds — not\n" +
	"milliseconds and not ISO dates. Flags are sent only when set, so Razorpay's\n" +
	"own defaults apply otherwise, and there is no auto-paging: raise `--skip` by\n" +
	"`--count` until `items` comes back short. Those four are the ONLY\n" +
	"server-side filters; nothing narrows by status, customer or amount, so wider\n" +
	"pages plus local filtering is the only route."

const (
	longPaymentList = "The time window filters on the payment's `created_at`, and it is the only\n" +
		"narrowing available — there is no filter by status, order, customer or\n" +
		"amount, so pull a wide page and sift `items` locally. Each entry carries\n" +
		"`order_id`, `method`, `status` (created, authorized, captured, refunded,\n" +
		"failed) and an `amount` in the smallest currency unit."

	longPaymentGet = "Takes a `pay_…` id. `amount_refunded` and `refund_status` sit on the payment\n" +
		"itself, so answering \"was this refunded\" needs no `refund list` call. The\n" +
		"`order_id` field links back to the order the payment fulfilled; `notes` holds\n" +
		"whatever key/value pairs the merchant's checkout attached."

	longOrderList = "An order is the merchant's INTENT to collect; the payment is the money that\n" +
		"arrived against it, so a paid order and its payment are two separate\n" +
		"records. `status` is created, attempted or paid, and `amount_paid` /\n" +
		"`amount_due` show how far it got. `--from` / `--to` filter on the order's\n" +
		"`created_at`."

	longOrderGet = "Takes an `order_id_…` id (Razorpay renders it `order_…` in the dashboard).\n" +
		"Returns the order's `amount`, `amount_paid`, `amount_due`, `receipt` and\n" +
		"`status`, all in the smallest currency unit. The payments raised against it\n" +
		"are NOT included — find them with `payment list` and match on `order_id`."

	longRefundList = "Refunds already issued, newest first. Nothing here creates one: there is no\n" +
		"refund verb in this tool, so a refund request has to be carried out in the\n" +
		"Razorpay dashboard. Each entry names the `payment_id` it reverses, its\n" +
		"`amount` in the smallest currency unit and its `speed_processed`\n" +
		"(`normal` or `optimum`)."

	longRefundGet = "Takes an `rfnd_…` id. `status` is pending, processed or failed, and it is the\n" +
		"refund's own progress, not the payment's — a processed refund can still take\n" +
		"days to reach the payer's bank. For the aggregate view of one payment,\n" +
		"`payment get` already carries `amount_refunded`."

	longCustomerList = "Only customers explicitly saved on the account appear here. A one-off\n" +
		"checkout does not create a customer record, so a payer may exist in\n" +
		"`payment list` with an email and contact and have no entry in this list at\n" +
		"all. There is no email or contact filter — page the collection and match\n" +
		"locally."

	longCustomerGet = "Takes a `cust_…` id. Returns the stored `name`, `email`, `contact` and\n" +
		"`notes` only; the customer's payments, orders and saved tokens are not\n" +
		"included and there is no verb that lists them by customer, so reach them\n" +
		"through `payment list` and match on the contact details."

	longPaymentLinkList = "Hosted links the merchant has already created — this tool cannot create or\n" +
		"cancel one, so a request to send someone a payment link cannot be met here.\n" +
		"`status` is created, partially_paid, expired, cancelled or paid, and\n" +
		"`short_url` is the link that was shared."

	longPaymentLinkGet = "Takes a `plink_…` id. Alongside `status` and `short_url` the entity carries\n" +
		"`amount_paid` and, once someone pays, the payment ids under `payments`,\n" +
		"which `payment get` then expands. `expire_by` is a Unix-second timestamp\n" +
		"like the list window's `--from` / `--to`."

	longSettlementList = "Settlements are the transfers of collected funds from Razorpay to the\n" +
		"merchant's own bank account, so this answers \"when did the money actually\n" +
		"land\" rather than \"who paid us\". `--from` / `--to` bound the settlement's\n" +
		"`created_at`. The per-transaction breakdown of what a settlement contains\n" +
		"lives on Razorpay's settlement-recon endpoint, which has no command here."

	longSettlementGet = "Takes a `setl_…` id and returns the settled `amount`, `fees`, `tax`,\n" +
		"`status` and `utr` — the bank reference to quote when reconciling against a\n" +
		"statement line. `amount` is net of fees and tax and, like everything else\n" +
		"here, is in the smallest currency unit."

	longSubscriptionList = "Recurring mandates, not individual charges: each cycle produces its own\n" +
		"payment that shows up in `payment list`. `status` runs created, authenticated,\n" +
		"active, pending, halted, cancelled, completed and expired, and `paid_count` /\n" +
		"`remaining_count` say how far through the plan it is. Subscriptions cannot be\n" +
		"created, paused or cancelled from here."

	longSubscriptionGet = "Takes a `sub_…` id. `current_start` and `current_end` bound the cycle being\n" +
		"billed now and `charge_at` is when the next attempt fires, all Unix seconds.\n" +
		"The plan behind `plan_id` is not fetchable through this tool — there is no\n" +
		"plan resource — so the amount per cycle has to come from the payments the\n" +
		"subscription has already raised."
)
