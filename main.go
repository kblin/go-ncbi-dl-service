package main

import (
	"bytes"
	"encoding/json"
	"github.com/gorilla/mux"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

type MoleculeType string

const (
	NUCLEOTIDE MoleculeType = "nucleotide"
	PROTEIN                 = "protein"
)

type DownloadJob struct {
	Accession    string       `json:"accession,omitempty"`
	CallbackId   string       `json:"callback_id,omitempty"`
	Email        string       `json:"email,omitempty"`
	Filename     string       `json:"filename,omitempty"`
	MoleculeType MoleculeType `json:"molecule_type,omitempty"`
}

const CallbackUrl string = "http://127.0.0.1:5020/api/v1.0/downloaded"
const NcbiUrl string = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/efetch.fcgi"

func (m *MoleculeType) UnmarshalText(b []byte) error {
	str := strings.Trim(string(b), `"`)

	switch str {
	case string(NUCLEOTIDE), string(PROTEIN):
		*m = MoleculeType(str)
	default:
		log.Printf("Invalid molecule_type '%s', returning 'nucleotide' instead\n", str)
		*m = NUCLEOTIDE
	}
	return nil
}

func DownloadAndCall(job *DownloadJob) {
	var httpClient = &http.Client{Timeout: time.Second * 10}
	var file_ending string

	req, err := http.NewRequest("GET", NcbiUrl, nil)
	if err != nil {
		log.Println(err)
		// TODO: Call the callback URL with a failure status
		return
	}

	q := req.URL.Query()
	q.Add("tool", "antiSMASH downloader")
	q.Add("retmode", "text")
	q.Add("id", job.Accession)

	switch job.MoleculeType {
	case NUCLEOTIDE:
		q.Add("db", "nucleotide")
		q.Add("rettype", "gbwithparts")
		file_ending = ".gbk"
	case PROTEIN:
		q.Add("db", "protein")
		q.Add("rettype", "fasta")
		file_ending = ".fa"
	default:
		log.Printf("Invalid molecule type %s, ignoring request.\n", job.MoleculeType)
		return
	}

	req.URL.RawQuery = q.Encode()

	outdir := path.Join(".", job.CallbackId)

	outfile := path.Join(outdir, job.Accession+file_ending)
	job.Filename = outfile

	if err := os.MkdirAll(outdir, os.ModePerm); err != nil {
		log.Println(err)
		return
	}
	out, err := os.Create(outfile)
	if err != nil {
		log.Println(err)
		return
	}
	defer out.Close()

	response, err := httpClient.Do(req)
	if err != nil {
		log.Println(err)
		return
	}
	defer response.Body.Close()

	_, err = io.Copy(out, response.Body)
	if err != nil {
		log.Println(err)
		return
	}

	body, _ := json.Marshal(job)
	response, err = httpClient.Post(CallbackUrl, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Println(err)
	}
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
}

func DownloadByAccessionHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	var job DownloadJob

	if err := json.NewDecoder(req.Body).Decode(&job); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	go DownloadAndCall(&job)
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(job)
}

func main() {
	router := mux.NewRouter()
	router.HandleFunc("/api/v1.0/download", DownloadByAccessionHandler).Methods("POST")
	log.Fatal(http.ListenAndServe(":5021", router))
}
