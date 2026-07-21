package meet

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// spaceConfigInput holds the raw config flag values shared by create/update.
type spaceConfigInput struct {
	accessType        string
	autoRecording     string
	autoTranscription string
	autoSmartNotes    string
	moderation        string
}

// addSpaceConfigFlags wires the shared space-config flags onto cmd.
func addSpaceConfigFlags(cmd *cobra.Command, in *spaceConfigInput) {
	cmd.Flags().StringVar(&in.accessType, "access-type", "", "who can join without knocking: open|trusted|restricted")
	cmd.Flags().StringVar(&in.autoRecording, "auto-recording", "", "auto-start recording: on|off")
	cmd.Flags().StringVar(&in.autoTranscription, "auto-transcription", "", "auto-start transcription: on|off")
	cmd.Flags().StringVar(&in.autoSmartNotes, "auto-smart-notes", "", "auto-start smart notes: on|off")
	cmd.Flags().StringVar(&in.moderation, "moderation", "", "host moderation: on|off")
}

// buildSpaceConfig turns the set flags into the SpaceConfig payload and the
// matching updateMask paths. Only flags the caller actually set are included,
// so unset fields keep their server/org defaults (and are absent from the mask
// on patch).
func buildSpaceConfig(cmd *cobra.Command, in spaceConfigInput) (map[string]any, []string, error) {
	config := map[string]any{}
	var mask []string
	artifact := map[string]any{}

	if cmd.Flags().Changed("access-type") {
		v, err := accessTypeValue(in.accessType)
		if err != nil {
			return nil, nil, err
		}
		config["accessType"] = v
		mask = append(mask, "config.accessType")
	}
	if cmd.Flags().Changed("auto-recording") {
		v, err := onOffValue("auto-recording", in.autoRecording)
		if err != nil {
			return nil, nil, err
		}
		artifact["recordingConfig"] = map[string]any{"autoRecordingGeneration": v}
		mask = append(mask, "config.artifactConfig.recordingConfig.autoRecordingGeneration")
	}
	if cmd.Flags().Changed("auto-transcription") {
		v, err := onOffValue("auto-transcription", in.autoTranscription)
		if err != nil {
			return nil, nil, err
		}
		artifact["transcriptionConfig"] = map[string]any{"autoTranscriptionGeneration": v}
		mask = append(mask, "config.artifactConfig.transcriptionConfig.autoTranscriptionGeneration")
	}
	if cmd.Flags().Changed("auto-smart-notes") {
		v, err := onOffValue("auto-smart-notes", in.autoSmartNotes)
		if err != nil {
			return nil, nil, err
		}
		artifact["smartNotesConfig"] = map[string]any{"autoSmartNotesGeneration": v}
		mask = append(mask, "config.artifactConfig.smartNotesConfig.autoSmartNotesGeneration")
	}
	if len(artifact) > 0 {
		config["artifactConfig"] = artifact
	}
	if cmd.Flags().Changed("moderation") {
		v, err := onOffValue("moderation", in.moderation)
		if err != nil {
			return nil, nil, err
		}
		config["moderation"] = v
		mask = append(mask, "config.moderation")
	}
	return config, mask, nil
}

// accessTypeValue and onOffValue match strictly (lowercase only, mirroring
// onedrive --scope): a non-canonical spelling fails at command validation
// instead of silently bypassing value-conditioned policy on the literal argv
// (fail-closed, design 318 §equals audit rule).
func accessTypeValue(v string) (string, error) {
	switch v {
	case "open":
		return "OPEN", nil
	case "trusted":
		return "TRUSTED", nil
	case "restricted":
		return "RESTRICTED", nil
	default:
		return "", fmt.Errorf("meet: --access-type must be open, trusted, or restricted (lowercase), got %q", v)
	}
}

func onOffValue(flag, v string) (string, error) {
	switch v {
	case "on":
		return "ON", nil
	case "off":
		return "OFF", nil
	default:
		return "", fmt.Errorf("meet: --%s must be on or off (lowercase), got %q", flag, v)
	}
}

func (s *Service) newSpacesGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <space | meeting-code>",
		Short: "Show a meeting space: URI, code, access + artifact config, active conference",
		Args:  cobra.ExactArgs(1),
		// GET /spaces/{s} — read-only (design 318).
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/"+spaceName(args[0]), nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			return s.renderSpace(body)
		},
	}
}

func (s *Service) newSpacesCreateCmd(token string) *cobra.Command {
	var in spaceConfigInput
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an ad-hoc meeting space (no calendar event); prints the meeting URI + code",
		Args:  cobra.NoArgs,
		// POST /spaces — mutating provider call (design 318).
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			config, _, err := buildSpaceConfig(cmd, in)
			if err != nil {
				return err
			}
			var payload any
			if len(config) > 0 {
				payload = map[string]any{"config": config}
			} else {
				payload = map[string]any{}
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/spaces", nil, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			return s.renderSpace(body)
		},
	}
	addSpaceConfigFlags(cmd, &in)
	return cmd
}

func (s *Service) newSpacesUpdateCmd(token string) *cobra.Command {
	var in spaceConfigInput
	cmd := &cobra.Command{
		Use:   "update <space>",
		Short: "Update a space's access/artifact config (spaces.patch; updateMask built from the set flags)",
		Args:  cobra.ExactArgs(1),
		// PATCH /spaces/{s} — mutating provider call (design 318).
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			config, mask, err := buildSpaceConfig(cmd, in)
			if err != nil {
				return err
			}
			if len(mask) == 0 {
				return fmt.Errorf("meet: nothing to update — pass --access-type, --auto-recording, --auto-transcription, --auto-smart-notes, or --moderation")
			}
			q := url.Values{}
			q.Set("updateMask", strings.Join(mask, ","))
			body, err := s.call(cmd.Context(), token, http.MethodPatch, "/"+spaceName(args[0]), q, map[string]any{"config": config})
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			fmt.Fprintf(s.stdout(), "updated %s (%s)\n", spaceName(args[0]), strings.Join(mask, ", "))
			return nil
		},
	}
	addSpaceConfigFlags(cmd, &in)
	return cmd
}

func (s *Service) newSpacesEndConferenceCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "end-conference <space>",
		Short: "End the active conference in a space — removes EVERYONE in the call (confirm with the user first)",
		Args:  cobra.ExactArgs(1),
		// POST /spaces/{s}:endActiveConference — mutating provider call
		// (design 318).
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/"+spaceName(args[0])+":endActiveConference", nil, map[string]any{})
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			fmt.Fprintf(s.stdout(), "ended active conference in %s\n", spaceName(args[0]))
			return nil
		},
	}
}

// renderSpace prints the human-readable summary of a Space resource body.
func (s *Service) renderSpace(body []byte) error {
	var sp struct {
		Name        string `json:"name"`
		MeetingURI  string `json:"meetingUri"`
		MeetingCode string `json:"meetingCode"`
		Config      struct {
			AccessType     string `json:"accessType"`
			ArtifactConfig struct {
				RecordingConfig struct {
					AutoRecordingGeneration string `json:"autoRecordingGeneration"`
				} `json:"recordingConfig"`
				TranscriptionConfig struct {
					AutoTranscriptionGeneration string `json:"autoTranscriptionGeneration"`
				} `json:"transcriptionConfig"`
				SmartNotesConfig struct {
					AutoSmartNotesGeneration string `json:"autoSmartNotesGeneration"`
				} `json:"smartNotesConfig"`
			} `json:"artifactConfig"`
			Moderation string `json:"moderation"`
		} `json:"config"`
		ActiveConference struct {
			ConferenceRecord string `json:"conferenceRecord"`
		} `json:"activeConference"`
	}
	if err := json.Unmarshal(body, &sp); err != nil {
		return fmt.Errorf("meet: decode space: %w", err)
	}
	active := sp.ActiveConference.ConferenceRecord
	if active == "" {
		active = "(none)"
	}
	fmt.Fprintf(s.stdout(),
		"Name:            %s\nMeetingUri:      %s\nMeetingCode:     %s\nAccessType:      %s\nAutoRecording:   %s\nAutoTranscript:  %s\nAutoSmartNotes:  %s\nModeration:      %s\nActiveConf:      %s\n",
		sp.Name, sp.MeetingURI, sp.MeetingCode, sp.Config.AccessType,
		sp.Config.ArtifactConfig.RecordingConfig.AutoRecordingGeneration,
		sp.Config.ArtifactConfig.TranscriptionConfig.AutoTranscriptionGeneration,
		sp.Config.ArtifactConfig.SmartNotesConfig.AutoSmartNotesGeneration,
		sp.Config.Moderation, active)
	return nil
}
