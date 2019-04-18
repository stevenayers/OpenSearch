package database

import (
	"clamber/conf"
	"clamber/page"
	"context"
	"encoding/json"
	"fmt"
	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"google.golang.org/grpc"
	"log"
	"strconv"
	"strings"
	"sync"
)

type (
	Store interface {
		SetSchema() (err error)
		DeleteAll() (err error)
		Create(currentPage *page.Page) (err error)
		FindNode(ctx *context.Context, txn *dgo.Txn, url string, depth int) (currentPage *page.Page, err error)
		FindOrCreateNode(ctx *context.Context, currentPage *page.Page) (uid string, err error)
		CheckPredicate(ctx *context.Context, txn *dgo.Txn, parentUid string, childUid string) (exists bool, err error)
		CheckOrCreatePredicate(ctx *context.Context, parentUid string, childUid string) (err error)
	}

	DbStore struct {
		*dgo.Dgraph
		Connection []*grpc.ClientConn
	}

	JsonPage struct {
		Uid       string      `json:"uid,omitempty"`
		Url       string      `json:"url,omitempty"`
		Timestamp int64       `json:"timestamp,omitempty"`
		Children  []*JsonPage `json:"links,omitempty"`
	}

	JsonPredicate struct {
		Matching int `json:"matching"`
	}
)

var DB Store

func InitStore(s *DbStore) {
	config := conf.GetConfig()
	var clients []api.DgraphClient
	for _, connConfig := range config.Database.Connections {
		connString := fmt.Sprintf("%s:%d", connConfig.Host, connConfig.Port)
		conn, err := grpc.Dial(connString, grpc.WithInsecure())
		if err != nil {
			fmt.Print(err)
		}
		clients = append(clients, api.NewDgraphClient(conn))
	}
	s.Dgraph = dgo.NewDgraphClient(clients...)
	DB = s
}

func SerializePage(currentPage *page.Page) (pb []byte, err error) {
	p := ConvertToJsonPage(currentPage)
	pb, err = json.Marshal(p)
	if err != nil {
		fmt.Print(err)
	}
	return
}

func DeserializePage(pb []byte) (currentPage *page.Page, err error) {
	jsonMap := make(map[string][]JsonPage)
	err = json.Unmarshal(pb, &jsonMap)
	jsonPages := jsonMap["result"]
	if len(jsonPages) > 0 {
		currentPage = ConvertToPage(nil, &jsonPages[0])
	}

	return
}

func DeserializePredicate(pb []byte) (exists bool, err error) {
	jsonMap := make(map[string][]JsonPredicate)
	err = json.Unmarshal(pb, &jsonMap)
	if err != nil {
		return
	}
	edges := jsonMap["edges"]
	if len(edges) > 0 {
		exists = edges[0].Matching > 0
	} else {
		exists = false
	}
	return
}

func ConvertToPage(parentPage *page.Page, jsonPage *JsonPage) (currentPage *page.Page) {
	currentPage = &page.Page{
		Uid:       jsonPage.Uid,
		Url:       jsonPage.Url,
		Timestamp: jsonPage.Timestamp,
	}
	if parentPage != nil {
		currentPage.Parent = parentPage
	}
	wg := sync.WaitGroup{}
	convertPagesChan := make(chan *page.Page)
	for _, childJsonPage := range jsonPage.Children {
		wg.Add(1)
		go func(childJsonPage *JsonPage) {
			defer wg.Done()
			childPage := ConvertToPage(currentPage, childJsonPage)
			convertPagesChan <- childPage
		}(childJsonPage)
	}
	go func() {
		wg.Wait()
		close(convertPagesChan)

	}()
	for childPages := range convertPagesChan {
		currentPage.Children = append(currentPage.Children, childPages)
	}
	return
}

func ConvertToJsonPage(currentPage *page.Page) (jsonPage JsonPage) {
	return JsonPage{
		Url:       currentPage.Url,
		Timestamp: currentPage.Timestamp,
	}
}

func (store *DbStore) SetSchema() (err error) {
	op := &api.Operation{}
	op.Schema = `
	url: string @index(exact) @upsert .
	timestamp: int .
    links: uid @count @reverse .
	`
	ctx := context.TODO()
	err = store.Alter(ctx, op)
	if err != nil {
		fmt.Print(err)
	}
	return
}

func (store *DbStore) DeleteAll() (err error) {
	err = store.Alter(context.Background(), &api.Operation{DropAll: true})
	return
}

func (store *DbStore) Create(currentPage *page.Page) (err error) {
	var currentUid string
	ctx := context.Background()
	currentUid, err = store.FindOrCreateNode(&ctx, currentPage)
	if err != nil {
		log.Printf("[ERROR] context: create current page (%s) - message: %s\n", currentPage.Url, err.Error())
		return
	}
	if currentPage.Parent != nil {
		var parentUid string
		parentUid, err = store.FindOrCreateNode(&ctx, currentPage.Parent)
		if err != nil {
			log.Printf("[ERROR] context: create parent page (%s) - message: %s\n", currentPage.Parent.Url, err.Error())
			return
		}
		err = store.CheckOrCreatePredicate(&ctx, parentUid, currentUid)
		if err != nil {
			log.Printf("[ERROR] create predicate (%s -> %s) - message: %s\n", parentUid, currentUid, err.Error())
			if !strings.Contains(err.Error(), "Transaction has been aborted. Please retry.") &&
				!strings.Contains(err.Error(), "Transaction is too old") {
				return
			}
		}
	}
	return
}

func (store *DbStore) FindNode(ctx *context.Context, txn *dgo.Txn, Url string, depth int) (currentPage *page.Page, err error) {
	queryDepth := strconv.Itoa(depth + 1)
	variables := map[string]string{"$url": Url}
	q := `query withvar($url: string, $depth: int){
			result(func: eq(url, $url)) @recurse(depth: ` + queryDepth + `, loop: false){
 				uid
				url
				timestamp
    			links
			}
		}`
	resp, err := txn.QueryWithVars(*ctx, q, variables)
	if err != nil {
		fmt.Print(err)
		return
	}
	currentPage, err = DeserializePage(resp.Json)
	return
}

func (store *DbStore) FindOrCreateNode(ctx *context.Context, currentPage *page.Page) (uid string, err error) {
	for uid == "" {
		var assigned *api.Assigned
		var p []byte
		var resultPage *page.Page
		txn := store.NewTxn()
		resultPage, err = store.FindNode(ctx, txn, currentPage.Url, 0)
		if err != nil {
			return
		} else if resultPage != nil {
			uid = resultPage.Uid
		}
		if uid == "" {
			p, err = SerializePage(currentPage)
			if err != nil {
				return
			}
			mu := &api.Mutation{}
			mu.SetJson = p
			assigned, err = txn.Mutate(*ctx, mu)
			if err != nil {
				return
			}
		}
		err = txn.Commit(*ctx)
		txn.Discard(*ctx)
		if uid == "" && err == nil {
			uid = assigned.Uids["blank-0"]
		}
		if uid != "" {
			currentPage.Uid = uid
		}

	}
	return
}

func (store *DbStore) CheckPredicate(ctx *context.Context, txn *dgo.Txn, parentUid string, childUid string) (exists bool, err error) {
	variables := map[string]string{"$parentUid": parentUid, "$childUid": childUid}
	q := `query withvar($parentUid: string, $childUid: string){
			edges(func: uid($parentUid)) {
				matching: count(links) @filter(uid($childUid))
			  }
			}`
	var resp *api.Response
	resp, err = txn.QueryWithVars(*ctx, q, variables)
	if err != nil {
		return
	}
	exists, err = DeserializePredicate(resp.Json)
	return
}

func (store *DbStore) CheckOrCreatePredicate(ctx *context.Context, parentUid string, childUid string) (err error) {
	attempts := 10
	exists := false
	for !exists && attempts > 0 {
		attempts--
		txn := store.NewTxn()
		exists, err = store.CheckPredicate(ctx, txn, parentUid, childUid)
		if err != nil {
			return
		}
		if !exists {
			_, err = txn.Mutate(*ctx, &api.Mutation{
				Set: []*api.NQuad{{
					Subject:   parentUid,
					Predicate: "links",
					ObjectId:  childUid,
				}}})
			if err != nil && attempts <= 0 {
				txn.Discard(*ctx)
				return
			}
			txn.Commit(*ctx)
			txn.Discard(*ctx)
		}
	}
	return
}
