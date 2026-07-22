package tiktok

// Default field selections. User defaults stay within the user.info.basic
// scope so a read call succeeds on a minimally-scoped sandbox token; callers
// request stats/profile fields explicitly via --fields once those scopes are
// granted. Video defaults cover the video.list scope.
const (
	defaultUserFields  = "open_id,union_id,avatar_url,display_name"
	defaultVideoFields = "id,title,video_description,duration,cover_image_url,create_time,share_url,like_count,comment_count,share_count,view_count"
)
