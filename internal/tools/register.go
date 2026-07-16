package tools

import (
	"github.com/heliohq/anycli/internal/tools/discord"
	"github.com/heliohq/anycli/internal/tools/figma"
	"github.com/heliohq/anycli/internal/tools/gmail"
	"github.com/heliohq/anycli/internal/tools/linkedin"
	"github.com/heliohq/anycli/internal/tools/microsoftcalendar"
	"github.com/heliohq/anycli/internal/tools/microsoftonedrive"
	"github.com/heliohq/anycli/internal/tools/microsoftoutlook"
	"github.com/heliohq/anycli/internal/tools/notion"
	"github.com/heliohq/anycli/internal/tools/slack"
	"github.com/heliohq/anycli/internal/tools/x"
)

// Built-in service registration. internal/exec imports this package (for
// GetService), so these init-time registrations are always live — no blank
// imports needed anywhere. Service packages implement the Service interface
// by duck typing and never import this registry, so registration cannot
// create an import cycle.
func init() {
	RegisterService("slack", &slack.Service{})
	RegisterService("notion", &notion.Service{})
	RegisterService("gmail", &gmail.Service{})
	RegisterService("discord", &discord.Service{})
	RegisterService("figma", &figma.Service{})
	RegisterService("linkedin", &linkedin.Service{})
	RegisterService("x", &x.Service{})
	RegisterService("microsoft-outlook", &microsoftoutlook.Service{})
	RegisterService("microsoft-calendar", &microsoftcalendar.Service{})
	RegisterService("microsoft-onedrive", &microsoftonedrive.Service{})
}
