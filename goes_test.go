// Copyright 2013 Belogik. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package goes

import (
	. "launchpad.net/gocheck"
	"net/url"
	"os"
	"testing"
	"time"
        "encoding/json"
)

var (
	ES_HOST = "localhost"
	ES_PORT = "9200"
)

// Hook up gocheck into the gotest runner.
func Test(t *testing.T) { TestingT(t) }

type GoesTestSuite struct{}

var _ = Suite(&GoesTestSuite{})

func (s *GoesTestSuite) SetUpTest(c *C) {
	h := os.Getenv("TEST_ELASTICSEARCH_HOST")
	if h != "" {
		ES_HOST = h
	}

	p := os.Getenv("TEST_ELASTICSEARCH_PORT")
	if p != "" {
		ES_PORT = p
	}
}

func (s *GoesTestSuite) TestNewConnection(c *C) {
	conn := NewConnection(ES_HOST, ES_PORT)
	c.Assert(conn, DeepEquals, &Connection{ES_HOST, ES_PORT})
}

func (s *GoesTestSuite) TestUrl(c *C) {
	conn := NewConnection(ES_HOST, ES_PORT)

	r := Request{
		Conn:      conn,
		Query:     "q",
		IndexList: []string{"i"},
		TypeList:  []string{},
		method:    "GET",
		api:       "_search",
	}

	c.Assert(r.Url(), Equals, "http://"+ES_HOST+":"+ES_PORT+"/i/_search")

	r.IndexList = []string{"a", "b"}
	c.Assert(r.Url(), Equals, "http://"+ES_HOST+":"+ES_PORT+"/a,b/_search")

	r.TypeList = []string{"c", "d"}
	c.Assert(r.Url(), Equals, "http://"+ES_HOST+":"+ES_PORT+"/a,b/c,d/_search")

	r.ExtraArgs = make(url.Values, 1)
	r.ExtraArgs.Set("version", "1")
	c.Assert(r.Url(), Equals, "http://"+ES_HOST+":"+ES_PORT+"/a,b/c,d/_search?version=1")

	r.id = "1234"
	r.api = ""
	c.Assert(r.Url(), Equals, "http://"+ES_HOST+":"+ES_PORT+"/a,b/c,d/1234/?version=1")
}

func (s *GoesTestSuite) TestEsDown(c *C) {
	conn := NewConnection("a.b.c.d", "1234")

	var query = map[string]interface{}{"query": "foo"}

	r := Request{
		Conn:      conn,
		Query:     query,
		IndexList: []string{"i"},
		method:    "GET",
		api:       "_search",
	}
	_, err := r.Run()

	c.Assert(err.Error(), Equals, "Get http://a.b.c.d:1234/i/_search: lookup a.b.c.d: no such host")
}

func (s *GoesTestSuite) TestRunMissingIndex(c *C) {
	conn := NewConnection(ES_HOST, ES_PORT)

	var query = map[string]interface{}{"query": "foo"}

	r := Request{
		Conn:      conn,
		Query:     query,
		IndexList: []string{"i"},
		method:    "GET",
		api:       "_search",
	}
	_, err := r.Run()

	c.Assert(err.Error(), Equals, "[404] IndexMissingException[[i] missing]")
}

func (s *GoesTestSuite) TestCreateIndex(c *C) {
	indexName := "testcreateindexgoes"
	conn := NewConnection(ES_HOST, ES_PORT)

	mapping := map[string]interface{}{
		"settings": map[string]interface{}{
			"index.number_of_shards":   1,
			"index.number_of_replicas": 0,
		},
		"mappings": map[string]interface{}{
			"_default_": map[string]interface{}{
				"_source": map[string]interface{}{
					"enabled": false,
				},
				"_all": map[string]interface{}{
					"enabled": false,
				},
			},
		},
	}

	resp, err := conn.CreateIndex(indexName, mapping)

	c.Assert(err, IsNil)
	c.Assert(resp.Ok, Equals, true)
	c.Assert(resp.Acknowledged, Equals, true)

	conn.DeleteIndex(indexName)

        raw, err := json.Marshal(mapping)
        c.Assert(err, IsNil)

	resp, err = conn.CreateIndex(indexName, string(raw))
	c.Assert(resp.Ok, Equals, true)
	c.Assert(resp.Acknowledged, Equals, true)
	conn.DeleteIndex(indexName)
}

func (s *GoesTestSuite) TestDeleteIndexInexistantIndex(c *C) {
	conn := NewConnection(ES_HOST, ES_PORT)
	resp, err := conn.DeleteIndex("foobar")

	c.Assert(err.Error(), Equals, "[404] IndexMissingException[[foobar] missing]")
	c.Assert(resp, DeepEquals, Response{})
}

func (s *GoesTestSuite) TestDeleteIndexExistingIndex(c *C) {
	conn := NewConnection(ES_HOST, ES_PORT)

	indexName := "testdeleteindexexistingindex"

	_, err := conn.CreateIndex(indexName, map[string]interface{}{})

	c.Assert(err, IsNil)

	resp, err := conn.DeleteIndex(indexName)
	c.Assert(err, IsNil)

	expectedResponse := Response{}
	expectedResponse.Ok = true
	expectedResponse.Acknowledged = true
	c.Assert(resp, DeepEquals, expectedResponse)
}

func (s *GoesTestSuite) TestRefreshIndex(c *C) {
	conn := NewConnection(ES_HOST, ES_PORT)
	indexName := "testrefreshindex"

	_, err := conn.CreateIndex(indexName, map[string]interface{}{})
	c.Assert(err, IsNil)

	resp, err := conn.RefreshIndex(indexName)
	c.Assert(err, IsNil)
	c.Assert(resp.Ok, Equals, true)

	_, err = conn.DeleteIndex(indexName)
	c.Assert(err, IsNil)
}

func (s *GoesTestSuite) TestBulkSend(c *C) {
	indexName := "testbulkadd"
	docType := "tweet"

	tweets := []Document{
		Document{
			Id:          "123",
			Index:       nil,
			Type:        docType,
			BulkCommand: BULK_COMMAND_INDEX,
			Fields: map[string]interface{}{
				"user":    "foo",
				"message": "some foo message",
			},
		},

		Document{
			Id:          nil,
			Index:       indexName,
			Type:        docType,
			BulkCommand: BULK_COMMAND_INDEX,
			Fields: map[string]interface{}{
				"user":    "bar",
				"message": "some bar message",
			},
		},
	}

	conn := NewConnection(ES_HOST, ES_PORT)

	_, err := conn.CreateIndex(indexName, nil)
	c.Assert(err, IsNil)

	response, err := conn.BulkSend(indexName, tweets)
	i := Item{
		Ok:      true,
		Id:      "123",
		Type:    docType,
		Version: 1,
		Index:   indexName,
	}
	c.Assert(response.Items[0][BULK_COMMAND_INDEX], Equals, i)
	c.Assert(err, IsNil)

	_, err = conn.RefreshIndex(indexName)
	c.Assert(err, IsNil)

	var query = map[string]interface{}{
		"query": map[string]interface{}{
			"match_all": map[string]interface{}{},
		},
	}

	searchResults, err := conn.Search(query, []string{indexName}, []string{})
	c.Assert(err, IsNil)

	var expectedTotal uint64 = 2
	c.Assert(searchResults.Hits.Total, Equals, expectedTotal)

	searchResultsRaw, err := conn.Search(`{"query":{"match_all":{}}}`, []string{indexName}, []string{})
	c.Assert(err, IsNil)
	c.Assert(searchResultsRaw.Hits.Total, Equals, expectedTotal)

	extraDocId := ""
	checked := 0
	for _, hit := range searchResults.Hits.Hits {
		if hit.Source["user"] == "foo" {
			c.Assert(hit.Id, Equals, "123")
			checked++
		}

		if hit.Source["user"] == "bar" {
			c.Assert(len(hit.Id) > 0, Equals, true)
			extraDocId = hit.Id
			checked++
		}
	}
	c.Assert(checked, Equals, 2)

	docToDelete := []Document{
		Document{
			Id:          "123",
			Index:       indexName,
			Type:        docType,
			BulkCommand: BULK_COMMAND_DELETE,
		},
		Document{
			Id:          extraDocId,
			Index:       indexName,
			Type:        docType,
			BulkCommand: BULK_COMMAND_DELETE,
		},
	}

	response, err = conn.BulkSend(indexName, docToDelete)
	i = Item{
		Ok:      true,
		Id:      "123",
		Type:    docType,
		Version: 2,
		Index:   indexName,
	}
	c.Assert(response.Items[0][BULK_COMMAND_DELETE], Equals, i)

	c.Assert(err, IsNil)

	_, err = conn.RefreshIndex(indexName)
	c.Assert(err, IsNil)

	searchResults, err = conn.Search(query, []string{indexName}, []string{})
	c.Assert(err, IsNil)

	expectedTotal = 0
	c.Assert(searchResults.Hits.Total, Equals, expectedTotal)

	_, err = conn.DeleteIndex(indexName)
	c.Assert(err, IsNil)
}

func (s *GoesTestSuite) TestStats(c *C) {
	conn := NewConnection(ES_HOST, ES_PORT)
	indexName := "teststats"

	conn.DeleteIndex(indexName)
	_, err := conn.CreateIndex(indexName, map[string]interface{}{})
	c.Assert(err, IsNil)

	// we must wait for a bit otherwise ES crashes
	time.Sleep(1 * time.Second)

	response, err := conn.Stats([]string{indexName}, url.Values{})
	c.Assert(err, IsNil)

	c.Assert(response.All.Indices[indexName].Primaries["docs"].Count, Equals, 0)

	_, err = conn.DeleteIndex(indexName)
	c.Assert(err, IsNil)
}

func (s *GoesTestSuite) TestIndexIdDefined(c *C) {
	indexName := "testindexiddefined"
	docType := "tweet"
	docId := "1234"

	conn := NewConnection(ES_HOST, ES_PORT)
	// just in case
	conn.DeleteIndex(indexName)

	_, err := conn.CreateIndex(indexName, map[string]interface{}{})
	c.Assert(err, IsNil)
	defer conn.DeleteIndex(indexName)

	d := Document{
		Index: indexName,
		Type:  docType,
		Id:    docId,
		Fields: map[string]interface{}{
			"user":    "foo",
			"message": "bar",
		},
	}

	extraArgs := make(url.Values, 1)
	extraArgs.Set("ttl", "86400000")
	response, err := conn.Index(d, extraArgs)
	c.Assert(err, IsNil)

	expectedResponse := Response{
		Ok:      true,
		Index:   indexName,
		Id:      docId,
		Type:    docType,
		Version: 1,
	}

	c.Assert(response, DeepEquals, expectedResponse)
}

func (s *GoesTestSuite) TestIndexIdNotDefined(c *C) {
	indexName := "testindexidnotdefined"
	docType := "tweet"

	conn := NewConnection(ES_HOST, ES_PORT)
	// just in case
	conn.DeleteIndex(indexName)

	_, err := conn.CreateIndex(indexName, map[string]interface{}{})
	c.Assert(err, IsNil)
	defer conn.DeleteIndex(indexName)

	d := Document{
		Index: indexName,
		Type:  docType,
		Fields: map[string]interface{}{
			"user":    "foo",
			"message": "bar",
		},
	}

	response, err := conn.Index(d, url.Values{})
	c.Assert(err, IsNil)

	c.Assert(response.Ok, Equals, true)
	c.Assert(response.Index, Equals, indexName)
	c.Assert(response.Type, Equals, docType)
	c.Assert(response.Version, Equals, 1)
	c.Assert(response.Id != "", Equals, true)
}

func (s *GoesTestSuite) TestDelete(c *C) {
	indexName := "testdelete"
	docType := "tweet"
	docId := "1234"

	conn := NewConnection(ES_HOST, ES_PORT)
	// just in case
	conn.DeleteIndex(indexName)

	_, err := conn.CreateIndex(indexName, map[string]interface{}{})
	c.Assert(err, IsNil)
	defer conn.DeleteIndex(indexName)

	d := Document{
		Index: indexName,
		Type:  docType,
		Id:    docId,
		Fields: map[string]interface{}{
			"user": "foo",
		},
	}

	_, err = conn.Index(d, url.Values{})
	c.Assert(err, IsNil)

	response, err := conn.Delete(d, url.Values{})
	c.Assert(err, IsNil)

	expectedResponse := Response{
		Ok:    true,
		Found: true,
		Index: indexName,
		Type:  docType,
		Id:    docId,
		// XXX : even after a DELETE the version number seems to be incremented
		Version: 2,
	}
	c.Assert(response, DeepEquals, expectedResponse)

	response, err = conn.Delete(d, url.Values{})
	c.Assert(err, IsNil)

	expectedResponse = Response{
		Ok:    true,
		Found: false,
		Index: indexName,
		Type:  docType,
		Id:    docId,
		// XXX : even after a DELETE the version number seems to be incremented
		Version: 3,
	}
	c.Assert(response, DeepEquals, expectedResponse)
}

func (s *GoesTestSuite) TestGet(c *C) {
	indexName := "testget"
	docType := "tweet"
	docId := "111"
	source := map[string]interface{}{
		"f1": "foo",
		"f2": "foo",
	}

	conn := NewConnection(ES_HOST, ES_PORT)
	conn.DeleteIndex(indexName)

	_, err := conn.CreateIndex(indexName, map[string]interface{}{})
	c.Assert(err, IsNil)
	defer conn.DeleteIndex(indexName)

	d := Document{
		Index:  indexName,
		Type:   docType,
		Id:     docId,
		Fields: source,
	}

	_, err = conn.Index(d, url.Values{})
	c.Assert(err, IsNil)

	response, err := conn.Get(indexName, docType, docId, url.Values{})
	c.Assert(err, IsNil)

	expectedResponse := Response{
		Index:   indexName,
		Type:    docType,
		Id:      docId,
		Version: 1,
		Exists:  true,
		Source:  source,
	}

	c.Assert(response, DeepEquals, expectedResponse)

	fields := make(url.Values, 1)
	fields.Set("fields", "f1")
	response, err = conn.Get(indexName, docType, docId, fields)
	c.Assert(err, IsNil)

	expectedResponse = Response{
		Index:   indexName,
		Type:    docType,
		Id:      docId,
		Version: 1,
		Exists:  true,
		Fields: map[string]interface{}{
			"f1": "foo",
		},
	}

	c.Assert(response, DeepEquals, expectedResponse)
}

func (s *GoesTestSuite) TestSearch(c *C) {
	indexName := "testsearch"
	docType := "tweet"
	docId := "1234"
	source := map[string]interface{}{
		"user":    "foo",
		"message": "bar",
	}

	conn := NewConnection(ES_HOST, ES_PORT)
	conn.DeleteIndex(indexName)

	_, err := conn.CreateIndex(indexName, map[string]interface{}{})
	c.Assert(err, IsNil)
	defer conn.DeleteIndex(indexName)

	d := Document{
		Index:  indexName,
		Type:   docType,
		Id:     docId,
		Fields: source,
	}

	_, err = conn.Index(d, url.Values{})
	c.Assert(err, IsNil)

	_, err = conn.RefreshIndex(indexName)
	c.Assert(err, IsNil)

	// I can feel my eyes bleeding
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					map[string]interface{}{
						"match_all": map[string]interface{}{},
					},
				},
			},
		},
	}
	response, err := conn.Search(query, []string{indexName}, []string{docType})

	expectedHits := Hits{
		Total:    1,
		MaxScore: 1.0,
		Hits: []Hit{
			Hit{
				Index:  indexName,
				Type:   docType,
				Id:     docId,
				Score:  1.0,
				Source: source,
			},
		},
	}

	c.Assert(response.Hits, DeepEquals, expectedHits)
}

func (s *GoesTestSuite) TestIndexStatus(c *C) {
	indexName := "testindexstatus"
	conn := NewConnection(ES_HOST, ES_PORT)
	conn.DeleteIndex(indexName)

	mapping := map[string]interface{}{
		"settings": map[string]interface{}{
			"index.number_of_shards":   1,
			"index.number_of_replicas": 1,
		},
	}

	_, err := conn.CreateIndex(indexName, mapping)
	c.Assert(err, IsNil)
	defer conn.DeleteIndex(indexName)

	// gives ES some time to do its job
	time.Sleep(1 * time.Second)

	response, err := conn.IndexStatus([]string{"_all"})
	c.Assert(err, IsNil)

	c.Assert(response.Ok, Equals, true)

	expectedShards := Shard{Total: 2, Successful: 1, Failed: 0}
	c.Assert(response.Shards, Equals, expectedShards)

	expectedIndices := map[string]IndexStatus{
		indexName: IndexStatus{
			Index: map[string]interface{}{
				"primary_size":          "99b",
				"primary_size_in_bytes": float64(99),
				"size":                  "99b",
				"size_in_bytes":         float64(99),
			},
			Translog: map[string]uint64{
				"operations": 0,
			},
			Docs: map[string]uint64{
				"num_docs":     0,
				"max_doc":      0,
				"deleted_docs": 0,
			},
			Merges: map[string]interface{}{
				"current":               float64(0),
				"current_docs":          float64(0),
				"current_size":          "0b",
				"current_size_in_bytes": float64(0),
				"total":                 float64(0),
				"total_time":            "0s",
				"total_time_in_millis":  float64(0),
				"total_docs":            float64(0),
				"total_size":            "0b",
				"total_size_in_bytes":   float64(0),
			},
			Refresh: map[string]interface{}{
				"total":                float64(1),
				"total_time":           "0s",
				"total_time_in_millis": float64(0),
			},
			Flush: map[string]interface{}{
				"total":                float64(0),
				"total_time":           "0s",
				"total_time_in_millis": float64(0),
			},
		},
	}

	c.Assert(response.Indices, DeepEquals, expectedIndices)
}
