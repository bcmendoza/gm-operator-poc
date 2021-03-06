package meshobjects

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dougfort/traversal"
)

func (c *Client) Ping() error {
	url := fmt.Sprintf("%s/zone", c.addr)

	var err error
	for i := 0; i < 5; i++ {
		if _, err := c.do(http.MethodGet, url, nil); err != nil {
			time.Sleep(time.Second * 1)
			continue
		}
		break
	}
	if err != nil {
		return errors.New("ping")
	}

	return nil
}

func (c *Client) Make(zoneKey, kind, key string, object json.RawMessage) error {
	url := fmt.Sprintf("%s/%s", c.addr, kind)

	body, err := c.do(http.MethodPost, url, object)
	if err != nil {
		return err
	}

	checksum := parseChecksum(
		traverse(
			traversal.Start(body).ObjectKey("result").ObjectKey("checksum"),
		),
	)
	if checksum == "" {
		return fmt.Errorf("no checksum returned from API")
	}

	kindTitle := strings.Title(kind)

	c.cache.Add(Revision{Mesh: zoneKey, Kind: kindTitle, Key: key}, checksum)
	c.logger.WithValues(kindTitle+"Key", key, "Checksum", checksum).Info("Configured " + kindTitle)
	return nil
}

func (c *Client) GetOrMake(zoneKey, kind, key string, object json.RawMessage) error {
	kindTitle := strings.Title(kind)
	if c.cache.Has(Revision{Mesh: zoneKey, Kind: kindTitle, Key: key}) {
		return nil
	}

	getUrl := fmt.Sprintf("%s/%s/%s", c.addr, kind, key)

	body, err := c.do(http.MethodGet, getUrl, nil)
	if err != nil {
		return err
	}

	bodyMap, err := traversal.GetMapFromRawMessage(body)
	if err != nil {
		return fmt.Errorf("traversal.GetMapFromRawMessage: %w", err)
	}

	if _, ok := bodyMap["result"]; ok {
		return nil
	}

	return c.Make(zoneKey, kind, key, object)
}

func (c *Client) Change(kind, key string, changes map[string]json.RawMessage) error {
	url := fmt.Sprintf("%s/%s/%s", c.addr, kind, key)

	body, err := c.do(http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	bodyMap, err := traversal.GetMapFromRawMessage(body)
	if err != nil {
		return fmt.Errorf("traversal.GetMapFromRawMessage: %w", err)
	}

	result, ok := bodyMap["result"]
	if !ok {
		return fmt.Errorf("GET %s did not have 'result', got %s", url, string(body))
	}

	object, err := traversal.GetMapFromRawMessage(result)
	if err != nil {
		return fmt.Errorf("traversal.GetMapFromRawMessage: %w", err)
	}

	for k, v := range changes {
		object[k] = v
	}

	updated, err := json.Marshal(object)
	if err != nil {
		return err
	}

	if _, err := c.do("PUT", url, updated); err != nil {
		return err
	}

	return nil
}
