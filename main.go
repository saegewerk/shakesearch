package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/blevesearch/bleve/analysis/tokenizer/unicode"
	"github.com/sajari/fuzzy"
	"index/suffixarray"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
)

func main() {
	searcher := Searcher{}

	err := searcher.Load("completeworks.txt")
	if err != nil {
		log.Fatal(err)
	}
	//

	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	http.HandleFunc("/search", handleSearch(searcher))

	port := os.Getenv("PORT")
	if port == "" {
		port = "3001"
	}

	fmt.Printf("Listening on port %s...", port)
	err = http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
	if err != nil {
		log.Fatal(err)
	}

}

type Searcher struct {
	CompleteWorks string
	SuffixArray   *suffixarray.Index
	Model         *fuzzy.Model
}
type Result struct {
	results []string
	Results []string
	Pagination
}
type Pagination struct {
	Pages  int
	Page   int
	Amount int
}
type PaginationDefaults struct {
	MaxAmount int
	Amount    int
}

var paginationDefaults = PaginationDefaults{
	MaxAmount: 20,
	Amount:    10,
}

func (result *Result) Paginate(amount, page int) error {
	print(amount * page)
	if len(result.results) < amount*page {
		return fmt.Errorf("Pages exceeded")
	} else if amount*page < 0 {
		return fmt.Errorf("Page sub-0")
	} else if amount <= 0 {
		return fmt.Errorf("0 amount")
	}
	result.Results = append(result.Results, "Showing Page "+strconv.Itoa(page)+" of "+strconv.Itoa(len(result.results)/amount))

	if amount*(page+1) > len(result.results) {
		result.Results = append(result.Results, result.results[amount*page:]...)
	} else {
		result.Results = append(result.Results, result.results[amount*page:amount*(page+1)]...)
	}
	return nil
}
func handleSearch(searcher Searcher) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		query, ok := r.URL.Query()["q"]
		if !ok || len(query[0]) < 1 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("missing search query in URL params"))
			return
		}
		results := Result{}

		results.results = searcher.Search(query[0])
		pagination := Pagination{
			Page:   0,
			Amount: 10,
		}
		page, okPage := r.URL.Query()["p"]
		if okPage {
			p, err := strconv.Atoi(page[0])
			if err == nil {
				pagination.Page = p - 1
			}
		}
		amount, okAmount := r.URL.Query()["a"]
		if okAmount {
			a, err := strconv.Atoi(amount[0])
			if err == nil {
				pagination.Amount = a
			}
		}

		if len(results.results) == 0 {
			spellchecked := searcher.Model.SpellCheck(query[0])
			results.Results = append(results.Results, "Nothing found showing results for "+spellchecked+" instead.")
			results.results = searcher.Search(spellchecked)
		}
		err := results.Paginate(pagination.Amount, pagination.Page)
		if err != nil {
			println(err.Error())
		}
		buf := &bytes.Buffer{}
		enc := json.NewEncoder(buf)
		err = enc.Encode(results.Results)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("encoding failure"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(buf.Bytes())
	}
}

func (s *Searcher) Load(filename string) error {
	dat, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("Load: %w", err)
	}
	s.CompleteWorks = string(dat)
	s.SuffixArray = suffixarray.New(dat)
	s.Model = fuzzy.NewModel()
	s.Model.SetDepth(3)
	t := unicode.NewUnicodeTokenizer()
	stream := t.Tokenize(dat)
	for i := 0; i < len(stream); i++ {
		s.Model.TrainWord(string(stream[i].Term))
	}
	return nil
}

func (s *Searcher) Search(query string) []string {

	idxs := s.SuffixArray.Lookup([]byte(query), -1)
	results := []string{}
	for _, idx := range idxs {
		if idx < 250 {
			results = append(results, s.CompleteWorks[0:idx+250])
		} else if idx > len(s.CompleteWorks)-250 {
			results = append(results, s.CompleteWorks[idx-250:len(s.CompleteWorks)])
		} else {
			results = append(results, s.CompleteWorks[idx-250:idx+250])
		}
	}
	return results
}
