package facebookpages

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// defaultPostFields is the feed projection returned by `post list` / `post get`.
const defaultPostFields = "id,message,created_time,permalink_url,status_type"

func (s *Service) newPostListCmd(token string) *cobra.Command {
	var fields, after string
	var limit int
	cmd := &cobra.Command{Use: "list", Short: "List a Page's posts (one page of the feed)", Args: cobra.NoArgs}
	cmd.Annotations = readOnly
	pageID := pageFlag(cmd)
	cmd.Flags().StringVar(&fields, "fields", "", "comma-separated Graph fields (default: post summary)")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum posts in this page (Graph default when 0)")
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor from a previous page's paging.cursors.after")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if limit < 0 {
			return &usageError{msg: "limit must not be negative"}
		}
		query := url.Values{"fields": {fieldsOrDefault(fields, defaultPostFields)}}
		if limit > 0 {
			query.Set("limit", strconv.Itoa(limit))
		}
		if after != "" {
			query.Set("after", after)
		}
		body, err := s.callAsPage(cmd.Context(), token, *pageID, http.MethodGet, "/"+url.PathEscape(*pageID)+"/feed", query, nil)
		if err != nil {
			return err
		}
		return s.emit(body)
	}
	return cmd
}

func (s *Service) newPostGetCmd(token string) *cobra.Command {
	var fields string
	cmd := &cobra.Command{Use: "get <post-id>", Short: "Get one post by id", Args: cobra.ExactArgs(1)}
	cmd.Annotations = readOnly
	pageID := pageFlag(cmd)
	cmd.Flags().StringVar(&fields, "fields", "", "comma-separated Graph fields (default: post summary)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		query := url.Values{"fields": {fieldsOrDefault(fields, defaultPostFields)}}
		body, err := s.callAsPage(cmd.Context(), token, *pageID, http.MethodGet, "/"+url.PathEscape(args[0]), query, nil)
		if err != nil {
			return err
		}
		return s.emit(body)
	}
	return cmd
}

func (s *Service) newPostCreateCmd(token string) *cobra.Command {
	var message, link string
	cmd := &cobra.Command{Use: "create", Short: "Publish a post to a Page (text and/or link)", Args: cobra.NoArgs}
	cmd.Annotations = writeAction
	pageID := pageFlag(cmd)
	cmd.Flags().StringVar(&message, "message", "", "post text")
	cmd.Flags().StringVar(&link, "link", "", "URL to attach")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if strings.TrimSpace(message) == "" && strings.TrimSpace(link) == "" {
			return &usageError{msg: "at least one of --message or --link is required"}
		}
		form := url.Values{}
		if message != "" {
			form.Set("message", message)
		}
		if link != "" {
			form.Set("link", link)
		}
		body, err := s.callAsPage(cmd.Context(), token, *pageID, http.MethodPost, "/"+url.PathEscape(*pageID)+"/feed", nil, form)
		if err != nil {
			return err
		}
		return s.emitCreated(body)
	}
	return cmd
}

func (s *Service) newPostUpdateCmd(token string) *cobra.Command {
	var message string
	cmd := &cobra.Command{Use: "update <post-id>", Short: "Edit a post's message", Args: cobra.ExactArgs(1)}
	cmd.Annotations = writeAction
	pageID := pageFlag(cmd)
	cmd.Flags().StringVar(&message, "message", "", "new post text")
	_ = cmd.MarkFlagRequired("message")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		form := url.Values{"message": {message}}
		body, err := s.callAsPage(cmd.Context(), token, *pageID, http.MethodPost, "/"+url.PathEscape(args[0]), nil, form)
		if err != nil {
			return err
		}
		return s.emit(body)
	}
	return cmd
}

func (s *Service) newPostDeleteCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "delete <post-id>", Short: "Delete a post", Args: cobra.ExactArgs(1)}
	cmd.Annotations = writeAction
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
