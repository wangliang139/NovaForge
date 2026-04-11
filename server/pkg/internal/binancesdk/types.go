package binancesdk

type WsAnnouncementEvent struct {
	CatalogId   int64  `json:"catalogId"`
	CatalogName string `json:"catalogName"`
	PublishDate int64  `json:"publishDate"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	Disclaimer  string `json:"disclaimer"`
}

type WsSapiEvent struct {
	Type  string `json:"type"`
	Topic string `json:"topic"`
	Data  string `json:"data"`
}
