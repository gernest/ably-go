package ably_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ably/ably-go/ably"
	"github.com/ably/ably-go/ably/ablytest"
	"github.com/ably/ably-go/ably/internal/ablyutil"
	"github.com/ably/ably-go/ably/proto"

	. "github.com/ably/ably-go/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/ably/ably-go/Godeps/_workspace/src/github.com/onsi/gomega"
)

func newHTTPClientMock(srv *httptest.Server) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) { return url.Parse(srv.URL) },
		},
	}
}

var _ = Describe("RestClient", func() {
	var (
		server *httptest.Server
	)
	Context("with a failing request", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(404)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"message":"Not Found"}`)
			}))

			options := &ably.ClientOptions{NoTLS: true, HTTPClient: newHTTPClientMock(server)}

			var err error
			client, err = ably.NewRestClient(testApp.Options(options))
			Expect(err).NotTo(HaveOccurred())

			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("encoding messages", func() {
		var (
			buffer   []byte
			server   *httptest.Server
			client   *ably.RestClient
			mockType string
			mockBody []byte
			err      error
		)

		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var err error
				buffer, err = ioutil.ReadAll(r.Body)
				Expect(err).NotTo(HaveOccurred())

				w.Header().Set("Content-Type", mockType)
				w.WriteHeader(200)
				w.Write(mockBody)
			}))

		})

		Context("with JSON encoding set up", func() {
			BeforeEach(func() {
				options := &ably.ClientOptions{
					NoTLS:            true,
					NoBinaryProtocol: true,
					HTTPClient:       newHTTPClientMock(server),
					AuthOptions: ably.AuthOptions{
						UseTokenAuth: true,
					},
				}

				mockType = "application/json"
				mockBody = []byte("{}")

				client, err = ably.NewRestClient(testApp.Options(options))
				Expect(err).NotTo(HaveOccurred())

				err := client.Channel("test").Publish("ping", "pong")
				Expect(err).NotTo(HaveOccurred())
			})

			It("encode the body of the message in JSON", func() {
				var anyJson []map[string]interface{}
				err := json.Unmarshal(buffer, &anyJson)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("with msgpack encoding set up", func() {
			BeforeEach(func() {
				options := &ably.ClientOptions{
					NoTLS:      true,
					HTTPClient: newHTTPClientMock(server),
					AuthOptions: ably.AuthOptions{
						UseTokenAuth: true,
					},
				}

				mockType = "application/x-msgpack"
				mockBody = []byte{0x80}

				options = testApp.Options(options)
				options.NoBinaryProtocol = false
				client, err = ably.NewRestClient(options)
				Expect(err).NotTo(HaveOccurred())

				err := client.Channel("test").Publish("ping", "pong")
				Expect(err).NotTo(HaveOccurred())
			})

			It("encode the body of the message using msgpack", func() {
				var anyMsgPack []map[string]interface{}
				err := ablyutil.Unmarshal(buffer, &anyMsgPack)
				Expect(err).NotTo(HaveOccurred())
				Expect(anyMsgPack[0]["name"]).To(Equal([]byte("ping")))
				Expect(anyMsgPack[0]["data"]).To(Equal([]byte("pong")))
			})
		})
	})

	Describe("Time", func() {
		It("returns srv time", func() {
			t, err := client.Time()
			Expect(err).NotTo(HaveOccurred())
			Expect(t.Unix()).To(BeNumerically("<=", time.Now().Add(2*time.Second).Unix()))
			Expect(t.Unix()).To(BeNumerically(">=", time.Now().Add(-2*time.Second).Unix()))
		})
	})

	Describe("Stats", func() {
		var lastInterval = time.Now().Add(-365 * 24 * time.Hour)
		var stats []*proto.Stats

		var jsonStats = `
			[
				{
					"inbound":{"realtime":{"messages":{"count":50,"data":5000}}},
					"outbound":{"realtime":{"messages":{"count":20,"data":2000}}}
				},
				{
					"inbound":{"realtime":{"messages":{"count":60,"data":6000}}},
					"outbound":{"realtime":{"messages":{"count":10,"data":1000}}}
				},
				{
					"inbound":{"realtime":{"messages":{"count":70,"data":7000}}},
					"outbound":{"realtime":{"messages":{"count":40,"data":4000}}},
					"persisted":{"presence":{"count":20,"data":2000}},
					"connections":{"tls":{"peak":20,"opened":10}},
					"channels":{"peak":50,"opened":30},
					"apiRequests":{"succeeded":50,"failed":10},
					"tokenRequests":{"succeeded":60,"failed":20}
				}
			]
		`

		BeforeEach(func() {
			err := json.NewDecoder(strings.NewReader(jsonStats)).Decode(&stats)
			Expect(err).NotTo(HaveOccurred())

			stats[0].IntervalID = proto.IntervalFormatFor(lastInterval.Add(-120*time.Minute), proto.StatGranularityMinute)
			stats[1].IntervalID = proto.IntervalFormatFor(lastInterval.Add(-60*time.Minute), proto.StatGranularityMinute)
			stats[2].IntervalID = proto.IntervalFormatFor(lastInterval.Add(-1*time.Minute), proto.StatGranularityMinute)

			res, err := client.Post("/stats", &stats, nil)
			Expect(err).NotTo(HaveOccurred())
			res.Body.Close()
		})

		It("parses stats from the rest api", func() {
			longAgo := lastInterval.Add(-120 * time.Minute)
			page, err := client.Stats(&ably.PaginateParams{
				Limit: 1,
				ScopeParams: ably.ScopeParams{
					Start: ably.Time(longAgo),
					Unit:  proto.StatGranularityMinute,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(page.Stats()[0].IntervalID).To(MatchRegexp("[0-9]+\\-[0-9]+\\-[0-9]+:[0-9]+:[0-9]+"))
		})
	})
})

func TestRSC7(t *testing.T) {
	app, err := ablytest.NewSandbox(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	c, err := ably.NewRestClient(app.Options())
	if err != nil {
		t.Fatal(err)
	}
	t.Run("must set version header", func(ts *testing.T) {
		req, err := c.NewHTTPRequest(&ably.Request{})
		if err != nil {
			ts.Fatal(err)
		}
		h := req.Header.Get(ably.AblyVersionHeader)
		if h != ably.AblyVersion {
			t.Errorf("expected %s got %s", ably.AblyVersion, h)
		}
	})
	t.Run("must set lib header", func(ts *testing.T) {
		req, err := c.NewHTTPRequest(&ably.Request{})
		if err != nil {
			ts.Fatal(err)
		}
		h := req.Header.Get(ably.AblyLibHeader)
		if h != ably.LibraryString {
			t.Errorf("expected %s got %s", ably.LibraryString, h)
		}
	})
}

func TestRest_hostfallback(t *testing.T) {
	app, err := ablytest.NewSandbox(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	runTestServer := func(ts *testing.T, options *ably.ClientOptions) (int, []string) {
		var retryCount int
		var hosts []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hosts = append(hosts, r.Host)
			retryCount++
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()
		options.HTTPClient = newHTTPClientMock(server)
		client, err := ably.NewRestClient(app.Options(options))
		if err != nil {
			ts.Fatal(err)
		}
		err = client.Channel("test").Publish("ping", "pong")
		if err == nil {
			ts.Error("expected an error")
		}
		return retryCount, hosts
	}
	t.Run("RSC15d RSC15a must use alternative host", func(ts *testing.T) {
		options := &ably.ClientOptions{
			NoTLS: true,
			AuthOptions: ably.AuthOptions{
				UseTokenAuth: true,
			},
		}
		retryCount, hosts := runTestServer(ts, options)
		if retryCount != 4 {
			ts.Fatalf("expected 4 http calls got %d", retryCount)
		}
		// make sure the host header is set. Since we are using defaults from the spec
		// the hosts should be in [a..e].ably-realtime.com
		expect := strings.Join(ably.DefaultFallbackHosts(), ", ")
		for _, host := range hosts[1:] {
			if !strings.Contains(expect, host) {
				ts.Errorf("expected %s got be in %s", host, expect)
			}
		}

		// ensure all picked fallbacks are unique
		uniq := make(map[string]bool)
		for _, h := range hosts {
			if _, ok := uniq[h]; ok {
				ts.Errorf("duplicate fallback %s", h)
			} else {
				uniq[h] = true
			}
		}
	})
	t.Run("rsc15b", func(ts *testing.T) {
		ts.Run("must not occur when default  rest.ably.io is overriden", func(ts *testing.T) {
			customHost := "example.com"
			options := &ably.ClientOptions{
				NoTLS:    true,
				RestHost: customHost,
				AuthOptions: ably.AuthOptions{
					UseTokenAuth: true,
				},
			}
			retryCount, hosts := runTestServer(ts, options)
			if retryCount != 1 {
				ts.Fatalf("expected 1 http call got %d", retryCount)
			}
			host := hosts[0]
			if host != customHost {
				ts.Errorf("expected %s got %s", customHost, host)
			}
		})
		ts.Run("must occur when fallbackHostsUseDefault is true", func(ts *testing.T) {
			customHost := "example.com"
			options := &ably.ClientOptions{
				NoTLS:                   true,
				RestHost:                customHost,
				FallbackHostsUseDefault: true,
				AuthOptions: ably.AuthOptions{
					UseTokenAuth: true,
				},
			}
			retryCount, hosts := runTestServer(ts, options)
			if retryCount != 4 {
				ts.Fatalf("expected 4 http call got %d", retryCount)
			}
			expect := strings.Join(ably.DefaultFallbackHosts(), ", ")
			for _, host := range hosts[1:] {
				if !strings.Contains(expect, host) {
					t.Errorf("expected %s got be in %s", host, expect)
				}
			}
		})
		ts.Run("must occur when fallbackHosts is set", func(ts *testing.T) {
			customHost := "example.com"
			fallback := "a.example.com"
			options := &ably.ClientOptions{
				NoTLS:         true,
				RestHost:      customHost,
				FallbackHosts: []string{fallback},
				AuthOptions: ably.AuthOptions{
					UseTokenAuth: true,
				},
			}
			retryCount, hosts := runTestServer(ts, options)
			if retryCount != 2 {
				ts.Fatalf("expected 2 http call got %d", retryCount)
			}
			host := hosts[1]
			if host != fallback {
				t.Errorf("expected %s got %s", fallback, host)
			}
		})
	})
	t.Run("RSC15e must start with default host", func(ts *testing.T) {
		options := &ably.ClientOptions{
			NoTLS: true,
			AuthOptions: ably.AuthOptions{
				UseTokenAuth: true,
			},
		}
		retryCount, hosts := runTestServer(ts, options)
		if retryCount != 4 {
			ts.Fatalf("expected 4 http calls got %d", retryCount)
		}
		firstHostCalled := hosts[0]
		if !strings.HasSuffix(firstHostCalled, ably.RestHost) {
			ts.Errorf("expected primary host got %s", firstHostCalled)
		}
	})
	t.Run("must not occur when FallbackHosts is an empty array", func(ts *testing.T) {
		customHost := "example.com"
		options := &ably.ClientOptions{
			NoTLS:         true,
			RestHost:      customHost,
			FallbackHosts: []string{},
			AuthOptions: ably.AuthOptions{
				UseTokenAuth: true,
			},
		}
		retryCount, _ := runTestServer(ts, options)
		if retryCount != 1 {
			ts.Fatalf("expected 1 http calls got %d", retryCount)
		}
	})
}

func TestRestChannels_RSN1(t *testing.T) {
	app, err := ablytest.NewSandbox(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	client, err := ably.NewRestClient(app.Options())
	if err != nil {
		t.Fatal(err)
	}
	if client.Channels == nil {
		t.Errorf("expected Channels to be initialized")
	}
	sample := []struct {
		name string
	}{
		{name: "first_channel"},
		{name: "second_channel"},
		{name: "third_channel"},
	}

	t.Run("RSN3 RSN3a  must create new channels when they don't exist", func(ts *testing.T) {
		for _, v := range sample {
			client.Channels.Get(v.name)
		}
		size := client.Channels.Len()
		if size != len(sample) {
			ts.Errorf("expected %d got %d", len(sample), size)
		}
	})
	t.Run("RSN4 RSN4a must release channels", func(ts *testing.T) {
		for _, v := range sample {
			ch := client.Channels.Get(v.name)
			client.Channels.Release(ch)
		}
		size := client.Channels.Len()
		if size != 0 {
			ts.Errorf("expected 0 channels  got %d", size)
		}
	})
	t.Run("ensure no deadlock in Rage", func(ts *testing.T) {
		client.Channels.Range(func(name string, _ *ably.RestChannel) bool {
			n := client.Channels.Get(name + "_range")
			return client.Channels.Exists(n.Name)
		})
	})
}
