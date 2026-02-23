package cache

import "fmt"

func StationKey(id int) string            { return fmt.Sprintf("search:station:%d", id) }
func ProvinceKey(id int) string           { return fmt.Sprintf("search:province:%d", id) }
func OperatorKey(id int) string           { return fmt.Sprintf("search:operator:%d", id) }
func ClassKey(id int) string              { return fmt.Sprintf("search:class:%d", id) }
func IntegrationKey(id int) string        { return fmt.Sprintf("search:integration:%d", id) }
func WhiteLabelKey(domain string) string  { return fmt.Sprintf("search:wl:%s", domain) }
func AutopackKey(from, to string) string  { return fmt.Sprintf("search:autopack:%s:%s", from, to) }
func SearchStationKey(id int) string      { return fmt.Sprintf("search:search_station:%d", id) }
func SearchProvinceKey(id int) string     { return fmt.Sprintf("search:search_province:%d", id) }
