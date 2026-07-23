package facebookpages

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// defaultCommentFields is the projection returned by `comment list`.
const defaultCommentFields = "id,message,from,created_time,like_count"

func (s *Service) newCommentListCmd(token string) *cobra.Command {
	var fields string
	cmd := &cobra.Command{Use: "list <post-id>", Short: "List comments on a post", Args: cobra.ExactArgs(1)}
	pageID := pageFlag(cmd)
	cmd.Flags().StringVar(&fields, "fields", "", "comma-separated Graph fields (default: comment summary)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		query := url.Values{"fields": {fieldsOrDefault(fields, defaultCommentFields)}}
		body, err := s.callAsPage(cmd.Context(), token, *pageID, http.MethodGet, "/"+url.PathEscape(args[0])+"/comments", query, nil)
		if err != nil {
			return err
		}
		return s.emit(body)
	}
	return cmd
}

func (s *Service) newCommentReplyCmd(token string) *cobra.Command {
	var message string
	cmd := &cobra.Command{Use: "reply <comment-id>", Short: "Reply to a comment", Args: cobra.ExactArgs(1)}
	pageID := pageFlag(cmd)
	cmd.Flags().StringVar(&message, "message", "", "reply text")
	_ = cmd.MarkFlagRequired("message")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		form := url.Values{"message": {message}}
		body, err := s.callAsPage(cmd.Context(), token, *pageID, http.MethodPost, "/"+url.PathEscape(args[0])+"/comments", nil, form)
		if err != nil {
			return err
		}
		return s.emitCreated(body)
	}
	return cmd
}

func (s *Service) newCommentHideCmd(token string) *cobra.Command {
	var hidden bool
	cmd := &cobra.Command{Use: "hide <comment-id>", Short: "Hide or unhide a comment", Args: cobra.ExactArgs(1)}
	pageID := pageFlag(cmd)
	cmd.Flags().BoolVar(&hidden, "hidden", true, "true to hide, false to unhide")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		form := url.Values{"is_hidden": {strconv.FormatBool(hidden)}}
		body, err := s.callAsPage(cmd.Context(), token, *pageID, http.MethodPost, "/"+url.PathEscape(args[0]), nil, form)
		if err != nil {
			return err
		}
		return s.emit(body)
	}
	return cmd
}

func (s *Service) newCommentDeleteCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "delete <comment-id>", Short: "Delete a comment", Args: cobra.ExactArgs(1)}
	pageID := pageFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		body, err := s.callAsPage(cmd.Context(), token, *pageID, http.MethodDelete, "/"+url.PathEscape(args[0]), nil, nil)
		if err != nil {
			return err
		}
		return s.emit(body)
	}
	return cmd
}
