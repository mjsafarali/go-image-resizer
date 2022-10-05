package main

import (
	"bytes"
	"fmt"
	"github.com/h2non/bimg"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
)

var (
	PROXIED_SERVER = os.Getenv("S3_ADDR")
)

type transport struct {
	http.RoundTripper
}

func (t *transport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	fmt.Println("go-proxy trying... ", req.URL)

	//[#1]Check if the url has `w` and `h` and `f` query params
	width := req.URL.Query().Get("w")
	height := req.URL.Query().Get("h")
	force := req.URL.Query().Get("f")

	// Just pass the request if no query params added
	if width == "" && height == "" {
		resp, err = t.RoundTripper.RoundTrip(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == 200 {
			fmt.Println("go-proxy requested file already available... ", req.URL)
			// If found return without any modifications
			return resp, nil
		}
	}

	//[#2] Else extract the original image's url.
	// We are using `w` and `h` query params with orig filename to represent the new size
	var requestedUrl = req.URL.String()
	var _position = strings.LastIndex(requestedUrl, "?")
	if _position < 1 {
		fmt.Println("go-proxy serving original file... ", req.URL)
		return t.RoundTripper.RoundTrip(req)
	}
	var origImageUrl = requestedUrl[0:_position]
	var requestedUrlPath = req.URL.Path //#TODO: Should use for storing new file

	var (
		withInt     int
		heightInt   int
		forceResize bool
	)

	if width != "" {
		withInt, err = strconv.Atoi(width)
		if err != nil {
			// handle error
			fmt.Println(err)
			return
		}
	}
	if height != "" {
		heightInt, err = strconv.Atoi(height)
		if err != nil {
			// handle error
			fmt.Println(err)
			return
		}
	}
	if force != "" {
		if force == "true" {
			forceResize = true
		} else {
			forceResize = false
		}
	}

	//[#3]
	targetUrl, err := url.Parse(origImageUrl)
	if err != nil {
		return
	}
	// Modify the request to use the generated url pointing to the original file.
	req.URL = targetUrl

	fmt.Println("go-proxy fetching from original Image... ", targetUrl)
	//[#4] Retrieve original image by using modified request
	resp, err = t.RoundTripper.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	//[#5] If original image found do resize operation and update the response body
	// before returning, else just return response as such
	if resp.StatusCode == 200 {
		err = resizeImage(requestedUrlPath, resp, withInt, heightInt, forceResize)
		if err != nil {
			return nil, err
		}
	}

	fmt.Println("go-proxy serving resized response", resp.Status)
	return resp, nil
}

/* [#1] Resizes image using bimg libvips wrapper. */
func resizeImage(path string, resp *http.Response, width int, height int, forceResize bool) error {
	//[#2] Read the response bytes into memory and close the Reader
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	err = resp.Body.Close()
	if err != nil {
		return err
	}

	//[#3] Create image from bytes
	origImage := bimg.NewImage(b)

	//[#4] calculate relative height using aspect ratio
	origSize, _ := origImage.Size()

	if width != 0 && height == 0 {
		aspectRatio := float64(origSize.Height) / float64(origSize.Width)
		height = int(float64(width) * aspectRatio)
	} else if width == 0 && height != 0 {
		aspectRatio := float64(origSize.Width) / float64(origSize.Height)
		width = int(float64(height) * aspectRatio)
	}

	//[#5] Apply resize operation with given width and height
	var newImage []byte
	if forceResize == true {
		newImage, err = origImage.ForceResize(width, height)
		if err != nil {
			return err
		}
	} else {
		newImage, err = origImage.Resize(width, height)
		if err != nil {
			return err
		}
	}

	//[#6] Reset response body with new bytes[] after resizing for sending back to caller
	resp.Body = io.NopCloser(bytes.NewReader(newImage))
	//[#7] Re-adjust the content length as per resized image size
	resp.Header["Content-Length"] = []string{fmt.Sprint(len(newImage))}
	fmt.Println("go-proxy  resized image size: ", resp.ContentLength, "bytes")

	//[#8] optionally do a background upload to s3 without making the caller to wait
	//imageContentType := resp.Header.Get("Content-Type")

	go func() {
		//fmt.Println("go-proxy - background processing response in go routine", imageContentType)
		/*	This part may be implemented using a throttled channel or something to limit
			the maximum number of uploads running at a time.
		*/

		//fmt.Println(path)
		//fmt.Println(imageContentType)
		//var aa *helpers.S3Storage
		//aa = helpers.NewS3Storage()
		//aa.Upload(path, newImage, imageContentType)
		// s3storage.Upload(path, newImage, imageContentType)
	}()

	return nil
}

func main() {

	// set vips cache to 0 to avoid cgo wrapper (bimg) not releasing memory to os.
	// https://github.com/h2non/bimg/issues/241
	bimg.VipsCacheSetMax(0)
	bimg.VipsCacheSetMaxMem(0)

	// Retrieve the proxies server hostname/domain
	targetUrl, err := url.Parse(PROXIED_SERVER)
	if err != nil {
		return
	}

	// Initialize a reverse proxy and pass the actual backend server here
	proxy := httputil.NewSingleHostReverseProxy(targetUrl)
	proxy.Transport = &transport{http.DefaultTransport}

	// Handle all requests to your server using the proxy
	http.HandleFunc("/", ProxyRequestHandler(proxy))
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func ProxyRequestHandler(proxy *httputil.ReverseProxy) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// Continue serving the request
		proxy.ServeHTTP(w, r)
	}
}
