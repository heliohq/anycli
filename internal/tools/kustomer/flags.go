package kustomer

import "github.com/spf13/cobra"

// listFlags holds the pagination + arbitrary-filter flags shared by the GET
// list commands. They register locally on those commands only — never as global
// flags.
type listFlags struct {
	page     int
	pageSize int
	query    []string
}

// registerListFlags attaches --page / --page-size / --query to cmd and returns
// the bound values. --query is repeatable (key=value) for any filter the list
// endpoint accepts beyond pagination.
func registerListFlags(cmd *cobra.Command) *listFlags {
	lf := &listFlags{}
	cmd.Flags().IntVar(&lf.page, "page", 0, "page number (1-based)")
	cmd.Flags().IntVar(&lf.pageSize, "page-size", 0, "items per page")
	cmd.Flags().StringArrayVar(&lf.query, "query", nil, "additional filter as key=value (repeatable)")
	return lf
}

// registerBodyFlags attaches the raw-JSON body flags (--data / --file) shared by
// the write commands, returning pointers to the bound values.
func registerBodyFlags(cmd *cobra.Command) (data, file *string) {
	data = cmd.Flags().String("data", "", "request body as a raw JSON string")
	file = cmd.Flags().String("file", "", "read the request body JSON from a file")
	return data, file
}
