package tools

import (
	"github.com/heliohq/anycli/internal/tools/activecampaign"
	"github.com/heliohq/anycli/internal/tools/acuity"
	"github.com/heliohq/anycli/internal/tools/adobesign"
	"github.com/heliohq/anycli/internal/tools/adyen"
	"github.com/heliohq/anycli/internal/tools/ahrefs"
	"github.com/heliohq/anycli/internal/tools/amplitude"
	"github.com/heliohq/anycli/internal/tools/apollo"
	"github.com/heliohq/anycli/internal/tools/attio"
	"github.com/heliohq/anycli/internal/tools/beehiiv"
	"github.com/heliohq/anycli/internal/tools/billcom"
	"github.com/heliohq/anycli/internal/tools/bitly"
	"github.com/heliohq/anycli/internal/tools/bluesky"
	"github.com/heliohq/anycli/internal/tools/boldsign"
	"github.com/heliohq/anycli/internal/tools/braintree"
	"github.com/heliohq/anycli/internal/tools/braze"
	"github.com/heliohq/anycli/internal/tools/brevo"
	"github.com/heliohq/anycli/internal/tools/brex"
	"github.com/heliohq/anycli/internal/tools/buffer"
	"github.com/heliohq/anycli/internal/tools/calcom"
	"github.com/heliohq/anycli/internal/tools/calendar"
	"github.com/heliohq/anycli/internal/tools/calendly"
	"github.com/heliohq/anycli/internal/tools/chargebee"
	"github.com/heliohq/anycli/internal/tools/close"
	"github.com/heliohq/anycli/internal/tools/contacts"
	"github.com/heliohq/anycli/internal/tools/copper"
	"github.com/heliohq/anycli/internal/tools/courier"
	"github.com/heliohq/anycli/internal/tools/crisp"
	"github.com/heliohq/anycli/internal/tools/customerio"
	"github.com/heliohq/anycli/internal/tools/dataforseo"
	"github.com/heliohq/anycli/internal/tools/delighted"
	"github.com/heliohq/anycli/internal/tools/discord"
	"github.com/heliohq/anycli/internal/tools/docs"
	"github.com/heliohq/anycli/internal/tools/docusign"
	"github.com/heliohq/anycli/internal/tools/drive"
	"github.com/heliohq/anycli/internal/tools/dropboxsign"
	"github.com/heliohq/anycli/internal/tools/expensify"
	"github.com/heliohq/anycli/internal/tools/facebookpages"
	"github.com/heliohq/anycli/internal/tools/figma"
	"github.com/heliohq/anycli/internal/tools/fillout"
	"github.com/heliohq/anycli/internal/tools/forms"
	"github.com/heliohq/anycli/internal/tools/freshbooks"
	"github.com/heliohq/anycli/internal/tools/gateprobe"
	"github.com/heliohq/anycli/internal/tools/formstack"
	"github.com/heliohq/anycli/internal/tools/freshdesk"
	"github.com/heliohq/anycli/internal/tools/freshservice"
	"github.com/heliohq/anycli/internal/tools/front"
	"github.com/heliohq/anycli/internal/tools/fullstory"
	"github.com/heliohq/anycli/internal/tools/gmail"
	"github.com/heliohq/anycli/internal/tools/googleads"
	"github.com/heliohq/anycli/internal/tools/googleanalytics"
	"github.com/heliohq/anycli/internal/tools/gorgias"
	"github.com/heliohq/anycli/internal/tools/gumroad"
	"github.com/heliohq/anycli/internal/tools/helpscout"
	"github.com/heliohq/anycli/internal/tools/hootsuite"
	"github.com/heliohq/anycli/internal/tools/hotjar"
	"github.com/heliohq/anycli/internal/tools/hubspot"
	"github.com/heliohq/anycli/internal/tools/hunter"
	"github.com/heliohq/anycli/internal/tools/instagram"
	"github.com/heliohq/anycli/internal/tools/instantly"
	"github.com/heliohq/anycli/internal/tools/intercom"
	"github.com/heliohq/anycli/internal/tools/iterable"
	"github.com/heliohq/anycli/internal/tools/jotform"
	"github.com/heliohq/anycli/internal/tools/keap"
	"github.com/heliohq/anycli/internal/tools/kit"
	"github.com/heliohq/anycli/internal/tools/klaviyo"
	"github.com/heliohq/anycli/internal/tools/knock"
	"github.com/heliohq/anycli/internal/tools/kustomer"
	"github.com/heliohq/anycli/internal/tools/later"
	"github.com/heliohq/anycli/internal/tools/lemlist"
	"github.com/heliohq/anycli/internal/tools/lemonsqueezy"
	"github.com/heliohq/anycli/internal/tools/linkedin"
	"github.com/heliohq/anycli/internal/tools/loops"
	"github.com/heliohq/anycli/internal/tools/lusha"
	"github.com/heliohq/anycli/internal/tools/mailchimp"
	"github.com/heliohq/anycli/internal/tools/mailerlite"
	"github.com/heliohq/anycli/internal/tools/mailjet"
	"github.com/heliohq/anycli/internal/tools/mastodon"
	"github.com/heliohq/anycli/internal/tools/meet"
	"github.com/heliohq/anycli/internal/tools/mercury"
	"github.com/heliohq/anycli/internal/tools/metaads"
	"github.com/heliohq/anycli/internal/tools/microsoftcalendar"
	"github.com/heliohq/anycli/internal/tools/microsoftonedrive"
	"github.com/heliohq/anycli/internal/tools/microsoftoutlook"
	"github.com/heliohq/anycli/internal/tools/missive"
	"github.com/heliohq/anycli/internal/tools/mixpanel"
	"github.com/heliohq/anycli/internal/tools/mongodb"
	"github.com/heliohq/anycli/internal/tools/moz"
	"github.com/heliohq/anycli/internal/tools/netsuite"
	"github.com/heliohq/anycli/internal/tools/notion"
	"github.com/heliohq/anycli/internal/tools/novu"
	"github.com/heliohq/anycli/internal/tools/omnisend"
	"github.com/heliohq/anycli/internal/tools/onesignal"
	"github.com/heliohq/anycli/internal/tools/outreach"
	"github.com/heliohq/anycli/internal/tools/paddle"
	"github.com/heliohq/anycli/internal/tools/pandadoc"
	"github.com/heliohq/anycli/internal/tools/paperform"
	"github.com/heliohq/anycli/internal/tools/paypal"
	"github.com/heliohq/anycli/internal/tools/pennylane"
	"github.com/heliohq/anycli/internal/tools/phantombuster"
	"github.com/heliohq/anycli/internal/tools/pinterest"
	"github.com/heliohq/anycli/internal/tools/pipedrive"
	"github.com/heliohq/anycli/internal/tools/plaid"
	"github.com/heliohq/anycli/internal/tools/posthog"
	"github.com/heliohq/anycli/internal/tools/postmark"
	"github.com/heliohq/anycli/internal/tools/quickbooks"
	"github.com/heliohq/anycli/internal/tools/ramp"
	"github.com/heliohq/anycli/internal/tools/razorpay"
	"github.com/heliohq/anycli/internal/tools/recurly"
	"github.com/heliohq/anycli/internal/tools/reddit"
	"github.com/heliohq/anycli/internal/tools/resend"
	"github.com/heliohq/anycli/internal/tools/rocketreach"
	"github.com/heliohq/anycli/internal/tools/sage"
	"github.com/heliohq/anycli/internal/tools/salesforce"
	"github.com/heliohq/anycli/internal/tools/salesloft"
	"github.com/heliohq/anycli/internal/tools/savvycal"
	"github.com/heliohq/anycli/internal/tools/searchconsole"
	"github.com/heliohq/anycli/internal/tools/segment"
	"github.com/heliohq/anycli/internal/tools/semrush"
	"github.com/heliohq/anycli/internal/tools/sendgrid"
	"github.com/heliohq/anycli/internal/tools/serpapi"
	"github.com/heliohq/anycli/internal/tools/servicenow"
	"github.com/heliohq/anycli/internal/tools/sheets"
	"github.com/heliohq/anycli/internal/tools/shopify"
	"github.com/heliohq/anycli/internal/tools/signnow"
	"github.com/heliohq/anycli/internal/tools/slack"
	"github.com/heliohq/anycli/internal/tools/slides"
	"github.com/heliohq/anycli/internal/tools/tasks"
	"github.com/heliohq/anycli/internal/tools/x"
)

// Built-in service registration. internal/exec imports this package (for
// GetService), so these init-time registrations are always live — no blank
// imports needed anywhere. Service packages implement the Service interface
// by duck typing and never import this registry, so registration cannot
// create an import cycle.
func init() {
	RegisterService("activecampaign", &activecampaign.Service{})
	RegisterService("acuity", &acuity.Service{})
	RegisterService("adyen", &adyen.Service{})
	RegisterService("ahrefs", &ahrefs.Service{})
	RegisterService("amplitude", &amplitude.Service{})
	RegisterService("apollo", &apollo.Service{})
	RegisterService("beehiiv", &beehiiv.Service{})
	RegisterService("bitly", &bitly.Service{})
	RegisterService("adobe-sign", &adobesign.Service{})
	RegisterService("bill-com", &billcom.Service{})
	RegisterService("bluesky", &bluesky.Service{})
	RegisterService("boldsign", &boldsign.Service{})
	RegisterService("braintree", &braintree.Service{})
	RegisterService("braze", &braze.Service{})
	RegisterService("brevo", &brevo.Service{})
	RegisterService("brex", &brex.Service{})
	RegisterService("buffer", &buffer.Service{})
	RegisterService("calcom", &calcom.Service{})
	RegisterService("delighted", &delighted.Service{})
	RegisterService("slack", &slack.Service{})
	RegisterService("notion", &notion.Service{})
	RegisterService("close", &close.Service{})
	RegisterService("novu", &novu.Service{})
	RegisterService("omnisend", &omnisend.Service{})
	RegisterService("outreach", &outreach.Service{})
	RegisterService("pandadoc", &pandadoc.Service{})
	RegisterService("paperform", &paperform.Service{})
	RegisterService("paypal", &paypal.Service{})
	RegisterService("pinterest", &pinterest.Service{})
	RegisterService("pipedrive", &pipedrive.Service{})
	RegisterService("posthog", &posthog.Service{})
	RegisterService("razorpay", &razorpay.Service{})
	RegisterService("recurly", &recurly.Service{})
	RegisterService("sage", &sage.Service{})
	RegisterService("salesloft", &salesloft.Service{})
	RegisterService("sheets", &sheets.Service{})
	RegisterService("signnow", &signnow.Service{})
	RegisterService("gmail", &gmail.Service{})
	RegisterService("google-ads", &googleads.Service{})
	RegisterService("hootsuite", &hootsuite.Service{})
	RegisterService("kit", &kit.Service{})
	RegisterService("klaviyo", &klaviyo.Service{})
	RegisterService("slides", &slides.Service{})
	RegisterService("calendar", &calendar.Service{})
	RegisterService("calendly", &calendly.Service{})
	RegisterService("contacts", &contacts.Service{})
	RegisterService("docs", &docs.Service{})
	RegisterService("drive", &drive.Service{})
	RegisterService("discord", &discord.Service{})
	RegisterService("docusign", &docusign.Service{})
	RegisterService("facebook-pages", &facebookpages.Service{})
	RegisterService("figma", &figma.Service{})
	RegisterService("fillout", &fillout.Service{})
	RegisterService("formstack", &formstack.Service{})
	RegisterService("forms", &forms.Service{})
	RegisterService("front", &front.Service{})
	RegisterService("linkedin", &linkedin.Service{})
	RegisterService("meet", &meet.Service{})
	RegisterService("meta-ads", &metaads.Service{})
	RegisterService("tasks", &tasks.Service{})
	RegisterService("x", &x.Service{})
	RegisterService("microsoft-outlook", &microsoftoutlook.Service{})
	RegisterService("microsoft-calendar", &microsoftcalendar.Service{})
	RegisterService("microsoft-onedrive", &microsoftonedrive.Service{})
	RegisterService("missive", &missive.Service{})
	RegisterService("mongodb", &mongodb.Service{})
	RegisterService("chargebee", &chargebee.Service{})
	RegisterService("expensify", &expensify.Service{})
	RegisterService("freshbooks", &freshbooks.Service{})
	RegisterService("gumroad", &gumroad.Service{})
	RegisterService("lemon-squeezy", &lemonsqueezy.Service{})
	RegisterService("mastodon", &mastodon.Service{})
	RegisterService("mercury", &mercury.Service{})
	RegisterService("netsuite", &netsuite.Service{})
	RegisterService("paddle", &paddle.Service{})
	RegisterService("pennylane", &pennylane.Service{})
	RegisterService("plaid", &plaid.Service{})
	RegisterService("quickbooks", &quickbooks.Service{})
	RegisterService("ramp", &ramp.Service{})
	RegisterService("shopify", &shopify.Service{})
	// gate-probe is the approval-gate E2E harness (design 318): hidden,
	// credential-free, local-echo-only. Registered like every other service
	// so Inspect/lint/policy coverage traverse it; consumer-side visibility
	// is gated by the consumer, not here.
	RegisterService("gate-probe", &gateprobe.Service{})
	RegisterService("attio", &attio.Service{})
	RegisterService("copper", &copper.Service{})
	RegisterService("courier", &courier.Service{})
	RegisterService("crisp", &crisp.Service{})
	RegisterService("customer-io", &customerio.Service{})
	RegisterService("dataforseo", &dataforseo.Service{})
	RegisterService("dropbox-sign", &dropboxsign.Service{})
	RegisterService("freshdesk", &freshdesk.Service{})
	RegisterService("freshservice", &freshservice.Service{})
	RegisterService("fullstory", &fullstory.Service{})
	RegisterService("google-analytics", &googleanalytics.Service{})
	RegisterService("gorgias", &gorgias.Service{})
	RegisterService("help-scout", &helpscout.Service{})
	RegisterService("hotjar", &hotjar.Service{})
	RegisterService("hubspot", &hubspot.Service{})
	RegisterService("hunter", &hunter.Service{})
	RegisterService("instagram", &instagram.Service{})
	RegisterService("instantly", &instantly.Service{})
	RegisterService("intercom", &intercom.Service{})
	RegisterService("iterable", &iterable.Service{})
	RegisterService("jotform", &jotform.Service{})
	RegisterService("keap", &keap.Service{})
	RegisterService("knock", &knock.Service{})
	RegisterService("kustomer", &kustomer.Service{})
	RegisterService("later", &later.Service{})
	RegisterService("lemlist", &lemlist.Service{})
	RegisterService("loops", &loops.Service{})
	RegisterService("lusha", &lusha.Service{})
	RegisterService("mailchimp", &mailchimp.Service{})
	RegisterService("mailerlite", &mailerlite.Service{})
	RegisterService("mailjet", &mailjet.Service{})
	RegisterService("mixpanel", &mixpanel.Service{})
	RegisterService("moz", &moz.Service{})
	RegisterService("onesignal", &onesignal.Service{})
	RegisterService("phantombuster", &phantombuster.Service{})
	RegisterService("postmark", &postmark.Service{})
	RegisterService("reddit", &reddit.Service{})
	RegisterService("resend", &resend.Service{})
	RegisterService("rocketreach", &rocketreach.Service{})
	RegisterService("salesforce", &salesforce.Service{})
	RegisterService("savvycal", &savvycal.Service{})
	RegisterService("search-console", &searchconsole.Service{})
	RegisterService("segment", &segment.Service{})
	RegisterService("semrush", &semrush.Service{})
	RegisterService("sendgrid", &sendgrid.Service{})
	RegisterService("serpapi", &serpapi.Service{})
	RegisterService("servicenow", &servicenow.Service{})
}
