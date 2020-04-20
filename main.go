package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/Xuanwo/storage"
	"github.com/Xuanwo/storage/coreutils"
	"github.com/Xuanwo/storage/pkg/credential"
	"github.com/Xuanwo/storage/services"
	"github.com/Xuanwo/storage/services/qingstor"
	"github.com/Xuanwo/storage/types/pairs"
	"github.com/google/go-github/v30/github"
	"golang.org/x/oauth2"
)

const dataFile = "data.json"

var (
	data   *Releases
	client *github.Client
	store  storage.Storager
)

var repos = []string{
	"qsctl",
}

func main() {
	ctx := context.Background()

	setup(ctx)

	for _, v := range repos {
		listAssets(ctx, v, listReleases(ctx, v))
	}

	content, err := json.Marshal(data.data)
	if err != nil {
		log.Fatalf("json marshal: %v", err)
	}
	err = ioutil.WriteFile(dataFile, content, 0644)
	if err != nil {
		log.Fatalf("write file: %v", err)
	}
}

func setup(ctx context.Context) {
	// Setup data
	data = &Releases{}

	content, err := ioutil.ReadFile(dataFile)
	if err != nil {
		log.Fatalf("read file: %v", err)
	}
	err = json.Unmarshal(content, &data.data)
	if err != nil {
		log.Fatalf("json unmarshal: %v", err)
	}

	// Setup github client
	oc := oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")}))

	client = github.NewClient(oc)

	// Setup storage
	store, err = coreutils.OpenStorager(qingstor.Type,
		pairs.WithCredential(credential.MustNewHmac(os.Getenv("QINGSTOR_ACCESS_KEY"), os.Getenv("QINGSTOR_SECRET_KEY"))),
		pairs.WithName(os.Getenv("QINGSTOR_BUCKET_NAME")),
		pairs.WithLocation(os.Getenv("QINGSTOR_BUCKET_LOCATION")),
	)
	if err != nil {
		log.Fatalf("open storager: %v", err)
	}
}

func listReleases(ctx context.Context, repo string) *github.RepositoryRelease {
	release, _, err := client.Repositories.GetLatestRelease(ctx, "qingstor", repo)
	if err != nil {
		log.Fatalf("list releases: %v", err)
	}
	return release
}

func listAssets(ctx context.Context, repo string, release *github.RepositoryRelease) {
	meta, err := store.Metadata()
	if err != nil {
		log.Fatalf("storage meta: %v", err)
	}
	location, ok := meta.GetLocation()
	if !ok {
		log.Fatalf("storage doesn't know location")
	}

	// We will not upload more than 100 assets
	assets, _, err := client.Repositories.ListReleaseAssets(ctx, "qingstor", repo, release.GetID(), &github.ListOptions{PerPage: 100})
	if err != nil {
		log.Fatalf("list assets: %v", err)
	}

	for _, asset := range assets {
		wg := &sync.WaitGroup{}
		wg.Add(1)

		go func(asset *github.ReleaseAsset) {
			defer wg.Done()
			path := fmt.Sprintf("%s/%s/%s", repo, release.GetTagName(), asset.GetName())

			log.Printf("Check if file %s exists", path)
			_, err := store.Stat(path)
			if err != nil && errors.Is(err, services.ErrObjectNotExist) {
				log.Printf("File %s not exists, try to upload", path)
				downloadAndUpload(ctx, asset, path)

				err = nil
			}
			if err != nil {
				log.Fatalf("storage stat: %v", err)
			}

			log.Printf("File %s exists, try to update data", path)
			url := fmt.Sprintf("https://%s.%s.qingstor.com/%s", meta.Name, location, path)
			data.Add(repo, release.GetTagName(), asset.GetName(), url)
		}(asset)

		wg.Wait()
	}
}

func downloadAndUpload(ctx context.Context, asset *github.ReleaseAsset, path string) {
	tmp, err := ioutil.TempFile("", "release-*")
	if err != nil {
		log.Fatalf("get tempfile: %v", err)
	}
	defer os.Remove(tmp.Name())

	log.Printf("Downloading file %s", path)
	r, err := http.Get(asset.GetBrowserDownloadURL())
	if err != nil {
		log.Fatalf("get asset content: %v", err)
	}
	defer r.Body.Close()

	n, err := io.Copy(tmp, r.Body)
	if err != nil {
		log.Fatalf("write asset content to local: %v", err)
	}

	tmp.Sync()
	tmp.Seek(0, 0)

	log.Printf("Uploading file %s", path)
	err = store.Write(path, tmp, pairs.WithSize(n))
	if err != nil {
		log.Fatalf("upload to qingstor: %v", err)
	}

	return
}
