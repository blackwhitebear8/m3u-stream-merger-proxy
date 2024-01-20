package m3u

type StreamInfo struct {
	Title   string
	LogoURL string
	Group   string
	URLs    []StreamURL
}

type StreamURL struct {
	Used    bool
	Content string
}
