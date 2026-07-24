package x

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	numericIDPattern        = regexp.MustCompile(`^[0-9]{1,19}$`)
	usernamePattern         = regexp.MustCompile(`^[A-Za-z0-9_]{1,15}$`)
	dmConversationIDPattern = regexp.MustCompile(`^([0-9]{1,19}-[0-9]{1,19}|[0-9]{15,19})$`)
)

func requireNumericID(name, value string) error {
	if !numericIDPattern.MatchString(value) {
		return fmt.Errorf("%s must be a numeric X id", name)
	}
	return nil
}

func requireDMConversationID(value string) error {
	if !dmConversationIDPattern.MatchString(value) {
		return fmt.Errorf("conversation id must be 15-19 digits or two 1-19 digit user ids separated by a hyphen")
	}
	return nil
}

func requireUsername(value string) error {
	if !usernamePattern.MatchString(value) {
		return fmt.Errorf("username must contain 1-15 letters, numbers, or underscores")
	}
	return nil
}

func requireExactlyOne(firstName, firstValue, secondName, secondValue string) error {
	firstSet := strings.TrimSpace(firstValue) != ""
	secondSet := strings.TrimSpace(secondValue) != ""
	if firstSet == secondSet {
		return fmt.Errorf("exactly one of %s or %s is required", firstName, secondName)
	}
	return nil
}

func requireLimit(value, min, max int) error {
	if value < min || value > max {
		return fmt.Errorf("limit must be between %d and %d", min, max)
	}
	return nil
}

func requireOptionalNumericID(name, value string) error {
	if value == "" {
		return nil
	}
	return requireNumericID(name, value)
}

func requireConnectedUserAndPostID(userID, postID string) error {
	if err := requireConnectedUserID(userID); err != nil {
		return err
	}
	return requireNumericID("post id", postID)
}

func requireConnectedUserAndTargetID(userID, targetID string) error {
	if err := requireConnectedUserID(userID); err != nil {
		return err
	}
	return requireNumericID("target user id", targetID)
}

func requireConnectedUserID(userID string) error {
	if userID == "" {
		return fmt.Errorf("X_USER_ID is not set — reconnect X")
	}
	return requireNumericID("connected user id", userID)
}

// mediaCategories are the media_category values accepted by the v2 media
// upload APIs.
var mediaCategories = map[string]struct{}{
	"tweet_image":   {},
	"tweet_video":   {},
	"tweet_gif":     {},
	"dm_image":      {},
	"dm_video":      {},
	"dm_gif":        {},
	"amplify_video": {},
}

func requireMediaCategory(value string) error {
	if value == "" {
		return nil // derived from the file type
	}
	if _, ok := mediaCategories[value]; !ok {
		return fmt.Errorf("category must be one of tweet_image, tweet_video, tweet_gif, dm_image, dm_video, dm_gif, amplify_video")
	}
	return nil
}

func requireSortOrder(value string) error {
	if value != "" && value != "recency" && value != "relevancy" {
		return fmt.Errorf(`sort order must be "recency" or "relevancy"`)
	}
	return nil
}
