package pipedrive

import (
	"net/http"

	"github.com/spf13/cobra"
)

// Entity command families. Each newXGroup builds a resource (pure data) and
// assembles its cobra group from the generic op builders in resource.go. v2
// entities use cursor pagination + PATCH updates; the v1-only entities (leads,
// notes) use offset pagination, and notes update with PUT.

// The deal Longs. They sit here rather than in the generic op builders in
// resource.go because the builders are entity-agnostic while the pipeline/stage
// coupling, the missing delete verb and the 500-vs-100 limit split are not.
const (
	longDealList = "Filters are exact query params, not searches: --status takes\n" +
		"open|won|lost|deleted and --pipeline-id, --stage-id, --person-id, --org-id\n" +
		"and --owner-id all take numeric ids. Finding a deal by name is `deal\n" +
		"search --term`, which this command cannot do. Cursor-paged: carry\n" +
		"`additional_data.next_cursor` back in --cursor until it comes back null.\n" +
		"--limit tops out at 500 here."

	longDealGet = "Takes the integer deal id. The record carries stage_id, pipeline_id and\n" +
		"status, which is where the deal sits; the stage's NAME is not in it and\n" +
		"comes from `stage get <stage-id>`. Linked people and organizations appear\n" +
		"as ids rather than expanded objects, so `person get` / `org get` are one\n" +
		"more call each."

	longDealCreate = "--title is the field Pipedrive actually needs. --value is a number and\n" +
		"--currency its three-letter code, sent as two separate fields rather than\n" +
		"one string. --stage-id and --pipeline-id have to agree, since a stage\n" +
		"belongs to exactly one pipeline; omit both and the deal lands in the\n" +
		"account's default pipeline. Custom fields have no typed flag — put them in\n" +
		"--data keyed by their Pipedrive field hash."

	longDealUpdate = "A partial patch: only the flags passed are sent, so the whole record never\n" +
		"has to be re-sent and no flag here clears a field. Moving a deal is\n" +
		"--stage-id, and the stage must belong to that deal's own pipeline — check\n" +
		"with `stage list --pipeline-id <id>` first. Closing is --status won, or\n" +
		"--status lost with --lost-reason. This is the substitute for a delete\n" +
		"command, which deliberately does not exist."

	longDealSearch = "--term is required and needs at least 2 characters. It matches deal titles,\n" +
		"and --fields widens that to other named fields — this is the fuzzy path,\n" +
		"while `deal list` filters exactly. --limit caps at 100 here, lower than the\n" +
		"500 `deal list` allows, and paging is by --cursor. Searching several entity\n" +
		"types at once is the top-level `search --types`, one call instead of\n" +
		"several."
)

func (s *Service) newDealGroup(c *caller) *cobra.Command {
	r := resource{
		c: c, word: "deal", short: "Manage deals", path: "/api/v2/deals",
		paginate: paginateCursor,
		filters: []filterFlag{
			{"person-id", "person_id", "filter by linked person id"},
			{"org-id", "org_id", "filter by linked organization id"},
			{"pipeline-id", "pipeline_id", "filter by pipeline id"},
			{"stage-id", "stage_id", "filter by stage id"},
			{"status", "status", "filter by status (open|won|lost|deleted)"},
			{"owner-id", "owner_id", "filter by owner user id"},
		},
		longs: map[string]string{
			"list": longDealList, "get": longDealGet, "create": longDealCreate,
			"update": longDealUpdate, "search": longDealSearch,
		},
		fields: []fieldSpec{
			{"title", "title", fieldString, "deal title"},
			{"value", "value", fieldFloat, "deal monetary value"},
			{"currency", "currency", fieldString, "deal currency (e.g. USD)"},
			{"person-id", "person_id", fieldInt, "linked person id"},
			{"org-id", "org_id", fieldInt, "linked organization id"},
			{"pipeline-id", "pipeline_id", fieldInt, "pipeline id"},
			{"stage-id", "stage_id", fieldInt, "stage id (move the deal)"},
			{"status", "status", fieldString, "status: open|won|lost"},
			{"lost-reason", "lost_reason", fieldString, "reason when status=lost"},
			{"owner-id", "owner_id", fieldInt, "owner user id"},
		},
	}
	return r.group(r.listCmd(), r.getCmd(), r.createCmd(), r.updateCmd(), r.searchCmd())
}

// The person Longs. The email/phone array shape and the absence of any dedupe
// on create are person-specific, so they cannot live in the shared builders.
const (
	longPersonList = "Filters are exact: --org-id and --owner-id take numeric ids, and there is\n" +
		"no name or email filter here — looking somebody up by name is `person\n" +
		"search --term`. Cursor-paged: carry `additional_data.next_cursor` into\n" +
		"--cursor, with --limit up to 500."

	longPersonGet = "Takes the integer person id. Emails and phones come back as arrays of typed\n" +
		"objects rather than plain strings, so one person can carry several of each\n" +
		"with one marked primary — read the array, not a scalar field. The person's\n" +
		"deals are not embedded; list them with `deal list --person-id <id>`."

	longPersonCreate = "--name is the field that matters; --org-id links the person to an\n" +
		"organization that must already exist, and --owner-id assigns them. Emails\n" +
		"and phones have no typed flag and go through --data in Pipedrive's array\n" +
		"shape, e.g. `{\"emails\":[{\"value\":\"jane@acme.com\",\"primary\":true}]}`.\n" +
		"Nothing deduplicates on create, so a second person with the same name is\n" +
		"simply a second record — run `person search --term` first."

	longPersonUpdate = "A partial patch: only the flags passed are sent, so omitting --org-id\n" +
		"leaves the existing link alone rather than unlinking. Re-pointing a person\n" +
		"at a different organization means passing --org-id with the new id; there\n" +
		"is no unlink flag. Anything outside --name, --org-id and --owner-id goes\n" +
		"through --data."

	longPersonSearch = "--term is required and needs at least 2 characters, matched against name,\n" +
		"email, phone and notes unless --fields narrows it. --limit caps at 100 and\n" +
		"paging is by --cursor. This is the pre-flight for `person create`, which\n" +
		"will happily create a duplicate."
)

func (s *Service) newPersonGroup(c *caller) *cobra.Command {
	r := resource{
		c: c, word: "person", short: "Manage persons (contacts)", path: "/api/v2/persons",
		paginate: paginateCursor,
		filters: []filterFlag{
			{"org-id", "org_id", "filter by linked organization id"},
			{"owner-id", "owner_id", "filter by owner user id"},
		},
		longs: map[string]string{
			"list": longPersonList, "get": longPersonGet, "create": longPersonCreate,
			"update": longPersonUpdate, "search": longPersonSearch,
		},
		fields: []fieldSpec{
			{"name", "name", fieldString, "person name"},
			{"org-id", "org_id", fieldInt, "linked organization id"},
			{"owner-id", "owner_id", fieldInt, "owner user id"},
		},
	}
	return r.group(r.listCmd(), r.getCmd(), r.createCmd(), r.updateCmd(), r.searchCmd())
}

// The organization Longs.
const (
	longOrgList = "--owner-id is the only filter — there is no name filter, so finding an\n" +
		"organization by name is `org search --term`. Cursor-paged with --cursor and\n" +
		"--limit up to 500."

	longOrgGet = "Takes the integer organization id and returns the organization's own record\n" +
		"only: its people and deals are not embedded. Enumerate those with `person\n" +
		"list --org-id <id>` and `deal list --org-id <id>`."

	longOrgCreate = "--name is the field that matters; --address is a single free-text string,\n" +
		"not structured components. Nothing deduplicates on create, so `org search\n" +
		"--term` first unless a duplicate is intended. The id that comes back is\n" +
		"what --org-id takes on persons, deals, activities and notes."

	longOrgUpdate = "A partial patch: only the flags passed are sent. Renaming with --name\n" +
		"changes the organization everywhere it appears, since links are held by id\n" +
		"and are unaffected by the name. Fields beyond --name, --address and\n" +
		"--owner-id go through --data."

	longOrgSearch = "--term is required and needs at least 2 characters, matched against\n" +
		"organization names and addresses. --limit caps at 100 and paging is by\n" +
		"--cursor. The ids returned are what --org-id takes throughout the tool."
)

func (s *Service) newOrgGroup(c *caller) *cobra.Command {
	r := resource{
		c: c, word: "org", short: "Manage organizations (accounts)", path: "/api/v2/organizations",
		paginate: paginateCursor,
		filters: []filterFlag{
			{"owner-id", "owner_id", "filter by owner user id"},
		},
		longs: map[string]string{
			"list": longOrgList, "get": longOrgGet, "create": longOrgCreate,
			"update": longOrgUpdate, "search": longOrgSearch,
		},
		fields: []fieldSpec{
			{"name", "name", fieldString, "organization name"},
			{"owner-id", "owner_id", fieldInt, "owner user id"},
			{"address", "address", fieldString, "organization address"},
		},
	}
	return r.group(r.listCmd(), r.getCmd(), r.createCmd(), r.updateCmd(), r.searchCmd())
}

// The activity Longs. Activities are the one entity family here that really is
// deletable, and --done is a filter value on list but a bare boolean on the
// writes — both facts are activity-specific.
const (
	longActivityList = "--done is a query VALUE here and takes 0 or 1, unlike the bare boolean\n" +
		"--done on create and update. --deal-id, --person-id, --org-id and\n" +
		"--owner-id filter by link. There is no date-range filter, so a \"due this\n" +
		"week\" question means filtering the returned due_date yourself. Cursor-paged\n" +
		"with --cursor and --limit up to 500."

	longActivityGet = "Takes the integer activity id. due_date and due_time are separate fields:\n" +
		"an activity with a date and no time is an all-day item, not one scheduled\n" +
		"at midnight. Links come back as deal_id / person_id / org_id rather than\n" +
		"expanded records."

	longActivityCreate = "--subject and --type are the fields that matter, and --type is the activity\n" +
		"type KEY configured on the account (call, meeting, task, …), not its\n" +
		"display label. --due-date is YYYY-MM-DD and --due-time a separate HH:MM;\n" +
		"omitting the time creates an all-day activity. Link it with exactly the id\n" +
		"meant — --deal-id, --person-id or --org-id."

	longActivityUpdate = "A partial patch: only the flags passed are sent. --done is a bare boolean —\n" +
		"write `--done` or `--done=true`, never `--done true`, which is parsed as a\n" +
		"second positional argument and rejected before any request is built.\n" +
		"Marking an activity done creates no follow-up; schedule that with a second\n" +
		"`activity create`."

	longActivityDelete = "Activities really are deletable, unlike deals and people, and this removes\n" +
		"the record rather than closing it. To record that something happened, use\n" +
		"`activity update <id> --done` instead — a deleted activity leaves no trace\n" +
		"on the deal and nothing here restores it."
)

func (s *Service) newActivityGroup(c *caller) *cobra.Command {
	r := resource{
		c: c, word: "activity", short: "Manage activities (calls, meetings, tasks)", path: "/api/v2/activities",
		paginate: paginateCursor,
		filters: []filterFlag{
			{"deal-id", "deal_id", "filter by linked deal id"},
			{"person-id", "person_id", "filter by linked person id"},
			{"org-id", "org_id", "filter by linked organization id"},
			{"owner-id", "owner_id", "filter by owner user id"},
			{"done", "done", "filter by done state (0|1)"},
		},
		longs: map[string]string{
			"list": longActivityList, "get": longActivityGet, "create": longActivityCreate,
			"update": longActivityUpdate, "delete": longActivityDelete,
		},
		fields: []fieldSpec{
			{"subject", "subject", fieldString, "activity subject"},
			{"type", "type", fieldString, "activity type key (e.g. call, meeting, task)"},
			{"due-date", "due_date", fieldString, "due date (YYYY-MM-DD)"},
			{"due-time", "due_time", fieldString, "due time (HH:MM)"},
			{"deal-id", "deal_id", fieldInt, "linked deal id"},
			{"person-id", "person_id", fieldInt, "linked person id"},
			{"org-id", "org_id", fieldInt, "linked organization id"},
			{"owner-id", "owner_id", fieldInt, "owner user id"},
			{"done", "done", fieldBool, "mark the activity done"},
			{"note", "note", fieldString, "activity note"},
		},
	}
	return r.group(r.listCmd(), r.getCmd(), r.createCmd(), r.updateCmd(), r.deleteCmd())
}

// The lead Longs. Leads are v1: UUID ids, offset pagination, and a nested value
// object with no typed flag — none of which the shared builders can express.
const (
	longLeadList = "Leads are a v1 resource and page by OFFSET, not cursor: pass --start with\n" +
		"--limit and read `additional_data.pagination.more_items_in_collection` to\n" +
		"decide whether to continue. --archived-status takes archived, not_archived\n" +
		"or all; the API's own default hides archived leads. Lead ids are UUID\n" +
		"strings, unlike the integer ids everywhere else in this tool."

	longLeadGet = "Takes the lead's UUID string — an integer id from any other resource will\n" +
		"not resolve here. A lead's monetary value is a nested object\n" +
		"(`{\"amount\":…,\"currency\":…}`), which is why there is no --value flag on\n" +
		"create or update and why it has to go through --data."

	longLeadCreate = "--title is the field that matters; --person-id or --org-id links the lead to\n" +
		"a contact that must already exist. The value has no typed flag and goes\n" +
		"through --data as `{\"value\":{\"amount\":5000,\"currency\":\"USD\"}}`. A lead is\n" +
		"pre-pipeline and there is no convert-to-deal command here — qualifying one\n" +
		"means creating the deal separately with `deal create`."

	longLeadUpdate = "A partial patch over the v1 lead endpoint; only the flags passed are sent,\n" +
		"and the id is the lead's UUID. Labels, the archived state and the nested\n" +
		"value object have no typed flags and go through --data — archiving a spent\n" +
		"lead that way keeps the record, which deleting it does not."

	longLeadDelete = "Takes the lead's UUID. This removes the record outright and nothing here\n" +
		"restores it, so a lead that actually converted is better archived through\n" +
		"`lead update --data` — that keeps the trail from inbound interest to the\n" +
		"deal it became."
)

func (s *Service) newLeadGroup(c *caller) *cobra.Command {
	r := resource{
		c: c, word: "lead", short: "Manage leads (v1)", path: "/api/v1/leads",
		paginate: paginateOffset, updateMethod: http.MethodPatch,
		filters: []filterFlag{
			{"owner-id", "owner_id", "filter by owner user id"},
			{"person-id", "person_id", "filter by linked person id"},
			{"org-id", "organization_id", "filter by linked organization id"},
			{"archived-status", "archived_status", "filter by archived status (archived|not_archived|all)"},
		},
		longs: map[string]string{
			"list": longLeadList, "get": longLeadGet, "create": longLeadCreate,
			"update": longLeadUpdate, "delete": longLeadDelete,
		},
		fields: []fieldSpec{
			{"title", "title", fieldString, "lead title"},
			{"owner-id", "owner_id", fieldInt, "owner user id"},
			{"person-id", "person_id", fieldInt, "linked person id"},
			{"org-id", "organization_id", fieldInt, "linked organization id"},
		},
	}
	return r.group(r.listCmd(), r.getCmd(), r.createCmd(), r.updateCmd(), r.deleteCmd())
}

// The note Longs. Notes are v1, create is spelled `add`, update is a PUT, and
// --lead-id is a UUID while every other link id is an integer.
const (
	longNoteList = "A v1 resource paging by OFFSET: pass --start with --limit and read\n" +
		"`additional_data.pagination.more_items_in_collection`. The link filters\n" +
		"--deal-id, --person-id, --org-id and --lead-id are how the notes on one\n" +
		"record are read; without one this returns the whole account's notes.\n" +
		"--lead-id is a UUID string, the other three are integers."

	longNoteGet = "Takes the note's own integer id, which is not the id of the record it hangs\n" +
		"off — that id belongs to `note list --deal-id` and friends. The content\n" +
		"comes back as the stored HTML rather than as plain text."

	longNoteAdd = "The create verb here is `add`, not `create`. --content accepts HTML.\n" +
		"Attach the note with exactly the link intended: --deal-id, --person-id,\n" +
		"--org-id or --lead-id, where --lead-id is a UUID string and the rest are\n" +
		"integers. A note created with no link is stored unattached and is not\n" +
		"reachable from any record afterwards."

	longNoteUpdate = "Notes update with PUT rather than PATCH, but only the flags actually passed\n" +
		"are sent, so it still behaves as a partial edit. --content REPLACES the\n" +
		"whole body, so read it with `note get` first when only part of it should\n" +
		"change. Setting a different link flag re-points the note at another record."

	longNoteDelete = "Takes the note's own integer id, not the id of the record it is attached\n" +
		"to. There is no archive or soft-delete state for notes and nothing here\n" +
		"restores one."
)

func (s *Service) newNoteGroup(c *caller) *cobra.Command {
	r := resource{
		c: c, word: "note", short: "Manage notes (v1)", path: "/api/v1/notes",
		paginate: paginateOffset, createVerb: "add", updateMethod: http.MethodPut,
		filters: []filterFlag{
			{"deal-id", "deal_id", "filter by deal id"},
			{"person-id", "person_id", "filter by person id"},
			{"org-id", "org_id", "filter by organization id"},
			{"lead-id", "lead_id", "filter by lead id"},
		},
		longs: map[string]string{
			"list": longNoteList, "get": longNoteGet, "add": longNoteAdd,
			"update": longNoteUpdate, "delete": longNoteDelete,
		},
		fields: []fieldSpec{
			{"content", "content", fieldString, "note content (HTML supported)"},
			{"deal-id", "deal_id", fieldInt, "attach to deal id"},
			{"person-id", "person_id", fieldInt, "attach to person id"},
			{"org-id", "org_id", fieldInt, "attach to organization id"},
			{"lead-id", "lead_id", fieldString, "attach to lead id (UUID)"},
		},
	}
	return r.group(r.listCmd(), r.getCmd(), r.createCmd(), r.updateCmd(), r.deleteCmd())
}

// The pipeline and stage Longs. Both families are read-only, and the fact that
// a stage id is meaningful only inside its own pipeline is what makes reading
// them a prerequisite for moving a deal.
const (
	longPipelineList = "Read-only: pipelines cannot be created or changed through this tool. The\n" +
		"ids here are what --pipeline-id takes on deals and on `stage list`, and\n" +
		"reading them is the first step in interpreting any deal's position, since a\n" +
		"stage id means nothing outside its own pipeline."

	longPipelineGet = "Takes the integer pipeline id and returns the pipeline record; its stages\n" +
		"are NOT embedded. List them with `stage list --pipeline-id <id>`."

	longStageList = "Pass --pipeline-id. Stage ids belong to exactly one pipeline, and an\n" +
		"unfiltered list mixes every pipeline's stages together — which is how a\n" +
		"deal ends up patched into a stage that does not belong to it. Each stage\n" +
		"carries an order_nr, which is what puts them in pipeline order. Read-only:\n" +
		"stages cannot be created or edited here."

	longStageGet = "Takes the integer stage id and returns the stage with its pipeline_id —\n" +
		"the way to confirm a stage belongs to the pipeline a deal is actually in\n" +
		"before running `deal update --stage-id`."
)

func (s *Service) newPipelineGroup(c *caller) *cobra.Command {
	r := resource{
		c: c, word: "pipeline", short: "Read pipelines", path: "/api/v2/pipelines",
		paginate: paginateCursor,
		longs:    map[string]string{"list": longPipelineList, "get": longPipelineGet},
	}
	return r.group(r.listCmd(), r.getCmd())
}

func (s *Service) newStageGroup(c *caller) *cobra.Command {
	r := resource{
		c: c, word: "stage", short: "Read stages", path: "/api/v2/stages",
		paginate: paginateCursor,
		longs:    map[string]string{"list": longStageList, "get": longStageGet},
		filters: []filterFlag{
			{"pipeline-id", "pipeline_id", "filter stages by pipeline id"},
		},
	}
	return r.group(r.listCmd(), r.getCmd())
}
