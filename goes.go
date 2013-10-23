// Copyright 2013 Belogik. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package goes provides an API to access Elasticsearch.
package goes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

const (
	BULK_COMMAND_INDEX  = "index"
	BULK_COMMAND_DELETE = "delete"
)

func (err *SearchError) Error() string {
	return fmt.Sprintf("[%d] %s", err.StatusCode, err.Msg)
}

// NewConnection initiates a new Connection to an elasticsearch server
//
// This function is pretty useless for now but might be useful in a near future
// if wee need more features like connection pooling or load balancing.
func NewConnection(host string, port string) *Connection {
	return &Connection{host, port}
}

// CreateIndex creates a new index represented by a name and a mapping
func (c *Connection) CreateIndex(name string, mapping interface{}) (Response, error) {
	r := Request{
		Conn:      c,
		Query:     mapping,
		IndexList: []string{name},
		method:    "PUT",
	}

	return r.Run()
}

// DeleteIndex deletes an index represented by a name
func (c *Connection) DeleteIndex(name string) (Response, error) {
	r := Request{
		Conn:      c,
		IndexList: []string{name},
		method:    "DELETE",
	}

	return r.Run()
}

// RefreshIndex refreshes an index represented by a name
func (c *Connection) RefreshIndex(name string) (Response, error) {
	r := Request{
		Conn:      c,
		IndexList: []string{name},
		method:    "POST",
		api:       "_refresh",
	}

	return r.Run()
}

// Stats fetches statistics (_stats) for the current elasticsearch server
func (c *Connection) Stats(indexList []string, extraArgs url.Values) (Response, error) {
	r := Request{
		Conn:      c,
		IndexList: indexList,
		ExtraArgs: extraArgs,
		method:    "GET",
		api:       "_stats",
	}

	return r.Run()
}

// IndexStatus fetches the status (_status) for the indices defined in
// indexList. Use _all in indexList to get stats for all indices
func (c *Connection) IndexStatus(indexList []string) (Response, error) {
	r := Request{
		Conn:      c,
		IndexList: indexList,
		method:    "GET",
		api:       "_status",
	}

	return r.Run()
}

// Bulk adds multiple documents in bulk mode to the index for a given type
func (c *Connection) BulkSend(index string, documents []Document) (Response, error) {
	// We do not generate a traditionnal JSON here (often a one liner)
	// Elasticsearch expects one line of JSON per line (EOL = \n)
	// plus an extra \n at the very end of the document
	//
	// More informations about the Bulk JSON format for Elasticsearch:
	//
	// - http://www.elasticsearch.org/guide/reference/api/bulk.html
	//
	// This is quite annoying for us as we can not use the simple JSON
	// Marshaler available in Run().
	//
	// We have to generate this special JSON by ourselves which leads to
	// the code below.
	//
	// I know it is unreadable I must find an elegant way to fix this.

	bulkData := []byte{}
	for _, doc := range documents {
		header := map[string]interface{}{
			doc.BulkCommand: map[string]interface{}{
				"_index": doc.Index,
				"_type":  doc.Type,
				"_id":    doc.Id,
			},
		}

		temp, err := json.Marshal(header)
		if err != nil {
			return Response{}, err
		}

		temp = append(temp, '\n')
		bulkData = append(bulkData, temp[:]...)

		if len(doc.Fields) > 0 {
			fields := map[string]interface{}{}
			for fieldName, fieldValue := range doc.Fields {
				fields[fieldName] = fieldValue
			}

			temp, err = json.Marshal(fields)
			if err != nil {
				return Response{}, err
			}

			temp = append(temp, '\n')
			bulkData = append(bulkData, temp[:]...)
		}
	}

	r := Request{
		Conn:      c,
		IndexList: []string{index},
		method:    "POST",
		api:       "_bulk",
		bulkData:  bulkData,
	}

	return r.Run()
}

// Search executes a search query against an index
func (c *Connection) Search(query interface{}, indexList []string, typeList []string) (Response, error) {
	r := Request{
		Conn:      c,
		Query:     query,
		IndexList: indexList,
		TypeList:  typeList,
		method:    "POST",
		api:       "_search",
	}

	return r.Run()
}

// Get a typed document by its id
func (c *Connection) Get(index string, documentType string, id string, extraArgs url.Values) (Response, error) {
	r := Request{
		Conn:      c,
		IndexList: []string{index},
		method:    "GET",
		api:       documentType + "/" + id,
		ExtraArgs: extraArgs,
	}

	return r.Run()
}

// Index indexes a Document
// The extraArgs is a list of url.Values that you can send to elasticsearch as
// URL arguments, for example, to control routing, ttl, version, op_type, etc.
func (c *Connection) Index(d Document, extraArgs url.Values) (Response, error) {
	r := Request{
		Conn:      c,
		Query:     d.Fields,
		IndexList: []string{d.Index.(string)},
		TypeList:  []string{d.Type},
		ExtraArgs: extraArgs,
		method:    "POST",
	}

	if d.Id != nil {
		r.method = "PUT"
		r.id = d.Id.(string)
	}

	return r.Run()
}

// Delete deletes a Document d
// The extraArgs is a list of url.Values that you can send to elasticsearch as
// URL arguments, for example, to control routing.
func (c *Connection) Delete(d Document, extraArgs url.Values) (Response, error) {
	r := Request{
		Conn:      c,
		IndexList: []string{d.Index.(string)},
		TypeList:  []string{d.Type},
		ExtraArgs: extraArgs,
		method:    "DELETE",
		id:        d.Id.(string),
	}

	return r.Run()
}

// Run executes an elasticsearch Request. It converts data to Json, sends the
// request and return the Response obtained
func (req *Request) Run() (Response, error) {
	postData := []byte{}

	// XXX : refactor this
	if req.api == "_bulk" {
		postData = req.bulkData
	} else {
                if raw,ok := req.Query.(string); ok {
                    postData = []byte(raw)
                } else {
                    b, err := json.Marshal(req.Query)
                    if err != nil {
                            return Response{}, err
                    }
                    postData = b
                }
	}

	reader := bytes.NewReader(postData)

	client := http.DefaultClient

	newReq, err := http.NewRequest(req.method, req.Url(), reader)
	if err != nil {
		return Response{}, err
	}

	if req.method == "POST" || req.method == "PUT" {
		newReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := client.Do(newReq)
	if err != nil {
		return Response{}, err
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return Response{}, err
	}

	if resp.StatusCode > 201 && resp.StatusCode < 400 {
		return Response{}, errors.New(string(body))
	}

	esResp := new(Response)
	err = json.Unmarshal(body, &esResp)
	if err != nil {
		return Response{}, err
	}

	if esResp.Error != "" {
		return Response{}, &SearchError{esResp.Error, esResp.Status}
	}

	return *esResp, nil
}

// Url builds a Request for a URL
func (r *Request) Url() string {
	path := "/" + strings.Join(r.IndexList, ",")

	if len(r.TypeList) > 0 {
		path += "/" + strings.Join(r.TypeList, ",")
	}

	// XXX : for indexing documents using the normal (non bulk) API
	if len(r.api) == 0 && len(r.id) > 0 {
		path += "/" + r.id
	}

	path += "/" + r.api

	u := url.URL{
		Scheme:   "http",
		Host:     fmt.Sprintf("%s:%s", r.Conn.Host, r.Conn.Port),
		Path:     path,
		RawQuery: r.ExtraArgs.Encode(),
	}

	return u.String()
}
