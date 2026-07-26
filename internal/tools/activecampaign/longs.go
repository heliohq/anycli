package activecampaign

// The `Long` text for the leaves built by the generic constructors in
// commands.go lives here: `newSimpleListCmd` alone builds nine list leaves, so
// the prose that separates a campaign from a deal stage has to be passed in
// rather than written inside the builder.

const longRoot = "ActiveCampaign's v3 JSON is printed verbatim, so a list reply is more than\n" +
	"its rows: the resource array arrives alongside sideloaded related objects and\n" +
	"a `meta.total` giving the full match count regardless of page size.\n" +
	"\n" +
	"Paging is `--limit` (ActiveCampaign defaults to 20 and caps it at 100) plus\n" +
	"`--offset`, both omitted from the request when left at 0. Nothing auto-pages:\n" +
	"compare the rows returned against `meta.total` and ask for the next offset.\n" +
	"\n" +
	"`--query` is repeatable on every `list` and passes ActiveCampaign's own\n" +
	"filter syntax through untouched — `--query email=jane@example.com`,\n" +
	"`--query 'filters[status]=1'`, `--query search=jane`. It is the only\n" +
	"filtering there is; no command has typed filter flags.\n" +
	"\n" +
	"Objects refer to each other by id, and every id comes from a different\n" +
	"command: `list list` for lists, `tag list` for tags, `pipeline list` and\n" +
	"`stage list` for a deal's group and stage, `field list` for custom fields.\n" +
	"Nothing here accepts a name in place of an id, so resolve first.\n" +
	"\n" +
	"Auth is an account-scoped API key, not OAuth. A 401 means the stored key or\n" +
	"account URL is wrong and will stay wrong — reconnect instead of retrying.\n" +
	"ActiveCampaign allows 5 requests per second per account and answers 429 above\n" +
	"that, so pace bulk work instead of fanning out."

const (
	longContactList = "`--query email=<address>` is an exact lookup and the usual way to turn an\n" +
		"address into the contact id every other contact command needs;\n" +
		"`--query search=<text>` matches loosely across name and email instead. A\n" +
		"contact row carries `links` to its tags, lists and deals rather than the\n" +
		"objects themselves, and no command here follows those links."

	longContactGet = "Takes the numeric contact id, not an email address — reach the id with\n" +
		"`contact list --query email=<address>` first. Returns the core contact\n" +
		"fields; custom field values and tag associations are not inlined, so plan on\n" +
		"`field list` to interpret anything beyond name, email and phone."

	longContactCreate = "`--email` is what ActiveCampaign deduplicates on: creating with an address\n" +
		"that already exists is an error rather than an update, so `contact update`\n" +
		"is the path for a known contact. `--data` takes any other v3 contact field\n" +
		"as a JSON object and is merged OVER the typed flags, so a key repeated in\n" +
		"both wins from `--data`. Custom fields go there as\n" +
		"`{\"fieldValues\":[{\"field\":\"1\",\"value\":\"…\"}]}`, using ids from `field list`.\n" +
		"Creating a contact does not put it on any list — that is `contact subscribe`."

	longContactUpdate = "A partial PUT: fields left unset keep their current values, and only the\n" +
		"typed flags plus `--data` that were actually given are sent. `--data` is\n" +
		"merged over the typed flags. Changing `--email` retargets the contact record\n" +
		"itself, so it also changes where any active automation mails it."

	longContactDelete = "Permanent and immediate: ActiveCampaign keeps no trash for contacts, and the\n" +
		"contact's list memberships, tag associations and engagement history go with\n" +
		"it. Re-creating the same address afterwards produces a new contact with a new\n" +
		"id and no history. Unsubscribing with `contact subscribe --status 2` is the\n" +
		"reversible alternative when the intent is only to stop mailing someone."

	longContactSubscribe = "Both `--list` and `--contact` are ids, from `list list` and `contact list`.\n" +
		"`--status` defaults to 1, so a call that omits it SUBSCRIBES; pass\n" +
		"`--status 2` to unsubscribe. Subscribing can trigger any automation the list\n" +
		"has a subscribe trigger on, which means this single call can start sending\n" +
		"mail. Re-running it for an existing membership updates the status rather\n" +
		"than failing."

	longContactTag = "`--contact` and `--tag` are both ids — `tag list` for the tag, `tag create`\n" +
		"when it does not exist yet. The response is the new `contactTag`\n" +
		"association, whose own id is the ONLY handle `contact untag` accepts, so\n" +
		"keep it. Tagging is a common automation trigger, so this can set mail in\n" +
		"motion."

	longContactUntag = "Takes the id of the contactTag ASSOCIATION, which is what `contact tag`\n" +
		"returns — a tag id from `tag list` will address the wrong association or 404.\n" +
		"No command lists a contact's associations, so an association id not captured\n" +
		"at tagging time cannot be recovered through this tool."

	longContactAutomate = "Enrolls the contact at the start of the automation, and the automation begins\n" +
		"running immediately — every mail, tag and delay in it will fire for that\n" +
		"person. There is no un-enroll verb here. Both `--contact` and `--automation`\n" +
		"are ids, the latter from `automation list`."
)

const (
	longListList = "Lists are the audiences a contact can be subscribed to, and their ids are\n" +
		"what `contact subscribe --list` takes. Read-only here: there is no verb that\n" +
		"creates, renames or deletes a list. Sender name, address and opt-in settings\n" +
		"come back on each row, which is how to tell a real mailing audience apart\n" +
		"from an internal bucket."

	longListGet = "Takes the list id from `list list`. Returns the list's sender identity and\n" +
		"opt-in configuration, not its members — there is no command that pages a\n" +
		"list's contacts; approach it from the contact side instead."

	longTagList = "Tag ids for `contact tag --tag`. `tagType` separates contact tags from\n" +
		"template tags, and only contact tags are meaningful for `contact tag`. There\n" +
		"is no delete or rename verb for tags, so a mistaken tag is permanent from\n" +
		"here."

	longDealList = "Deals are the CRM pipeline objects, unrelated to lists and campaigns. Each\n" +
		"carries `group` (the pipeline), `stage`, `owner`, `contact` and a `value` in\n" +
		"cents of `currency`. Narrow with the provider's own filter syntax, e.g.\n" +
		"`--query 'filters[stage]=5'`. There is no delete verb for deals."

	longDealGet = "Takes the deal id. `value` is an integer in the smallest currency unit —\n" +
		"1000 with `currency` usd is $10.00, not $1000 — and `group` / `stage` are\n" +
		"the ids that `pipeline list` and `stage list` resolve to names."

	longDealCreate = "`--data` is the whole payload as a JSON object; there are no typed flags.\n" +
		"ActiveCampaign requires `title`, `value` (an integer in cents), `currency`,\n" +
		"the `group` pipeline id from `pipeline list` and a `stage` id from\n" +
		"`stage list`, and links the deal to a person with `contact`. Send the fields\n" +
		"bare — the `{\"deal\":…}` wrapper is added for you."

	longDealUpdate = "`--data` carries only the fields that change; the rest keep their values.\n" +
		"Moving a deal along the pipeline is this command with a new `stage` id from\n" +
		"`stage list`, and reassigning it is a new `owner`. Deals cannot be deleted\n" +
		"through this tool, so a dead deal is closed by moving it to a lost stage."

	longPipelineList = "ActiveCampaign calls a pipeline a dealGroup, which is why this reads from\n" +
		"`dealGroups`. The `id` of each row is the `group` field a deal payload\n" +
		"carries, and the stages belonging to it come from `stage list`. Read-only:\n" +
		"pipelines are built in ActiveCampaign itself."

	longStageList = "Stages are ActiveCampaign's dealStages. Each row's `group` says which\n" +
		"pipeline it belongs to — the list is not scoped to one pipeline, so filter\n" +
		"by that field after fetching. A stage `id` is what `deal create` and\n" +
		"`deal update` set as `stage` to place or move a deal."

	longCampaignList = "Reporting only: this reads campaigns that already exist, with their send,\n" +
		"open, click and bounce counters. Nothing in this tool composes, schedules or\n" +
		"sends a campaign, and there is no verb for the message content behind one."

	longCampaignGet = "Takes the campaign id. Returns the campaign's status, schedule and aggregate\n" +
		"counters. Per-recipient detail — who opened or clicked — is not reachable\n" +
		"from any command here, so a question about individuals has to be answered\n" +
		"from the contact side."

	longAutomationList = "The automation ids `contact automate --automation` needs, with each\n" +
		"automation's status and entered/exited counters. Read-only: automations are\n" +
		"authored in ActiveCampaign, and nothing here starts, pauses or edits one."

	longFieldList = "The custom contact field DEFINITIONS, not any contact's values. Each row's\n" +
		"`id` is what a contact write references as\n" +
		"`{\"fieldValues\":[{\"field\":\"<id>\",\"value\":\"…\"}]}` inside `--data`, and\n" +
		"`perstag` is the personalization tag the same field answers to inside\n" +
		"campaign content. Run this before writing custom data — a wrong field id is\n" +
		"accepted shape-wise and silently stores nothing useful."

	longAccountList = "Accounts are ActiveCampaign's B2B company records, a separate object from\n" +
		"contacts: a contact is linked to one through an accountContact association\n" +
		"that no command here creates or reads. Read-only — there is no create,\n" +
		"update or delete verb for accounts."
)

const (
	longTagCreate = "`--name` is required and becomes the `tag` field ActiveCampaign stores.\n" +
		"`--type` defaults to `contact`, which is the type `contact tag` can apply;\n" +
		"`template` tags exist for campaign content and are not interchangeable.\n" +
		"Creating a tag that already exists is rejected rather than reused, so check\n" +
		"`tag list` first. Tags cannot be deleted or renamed from here."
)
