package zoominfo

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func mustNow() time.Time { return time.Now() }

// testKeyPEM generates a fresh RSA key and returns its PKCS#8 PEM plus the
// public key so tests verify the RS256 client-assertion signature end to end.
func testKeyPEM(t *testing.T) (string, *rsa.PublicKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	return pemText, &key.PublicKey
}

func credEnv(t *testing.T, keyPEM string) map[string]string {
	t.Helper()
	blob, err := json.Marshal(credentials{Username: "rep@acme.com", ClientID: "cid-123", PrivateKey: keyPEM})
	if err != nil {
		t.Fatalf("marshal creds: %v", err)
	}
	return map[string]string{EnvCredentials: string(blob)}
}

// verifyAssertion checks the RS256 signature and decodes the claim set.
func verifyAssertion(t *testing.T, assertion string, pub *rsa.PublicKey) map[string]any {
	t.Helper()
	parts := strings.Split(assertion, ".")
	if len(parts) != 3 {
		t.Fatalf("assertion is not a 3-part JWT: %q", assertion)
	}
	signingInput := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	digest := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("RS256 signature verification failed: %v", err)
	}
	header, _ := base64.RawURLEncoding.DecodeString(parts[0])
	var h map[string]any
	if err := json.Unmarshal(header, &h); err != nil {
		t.Fatalf("decode header: %v", err)
	}
	if h["alg"] != "RS256" || h["typ"] != "JWT" {
		t.Fatalf("unexpected JWT header: %v", h)
	}
	claims, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var c map[string]any
	if err := json.Unmarshal(claims, &c); err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	return c
}

// fakeServer serves /authenticate (asserting the Bearer client-assertion) and
// the data endpoints (asserting the Bearer access JWT). It records the last
// data request body.
type fakeServer struct {
	pub          *rsa.PublicKey
	accessJWT    string
	dataStatus   int
	dataResponse string
	lastAuthHdr  string
	lastBody     []byte
	lastPath     string
	lastMethod   string
	assertion    map[string]any
}

func (f *fakeServer) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/authenticate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("authenticate method = %s, want POST", r.Method)
		}
		bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		f.assertion = verifyAssertion(t, bearer, f.pub)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jwt":"` + f.accessJWT + `"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		f.lastAuthHdr = r.Header.Get("Authorization")
		f.lastPath = r.URL.Path
		f.lastMethod = r.Method
		f.lastBody, _ = readAllBody(r)
		status := f.dataStatus
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(f.dataResponse))
	})
	return mux
}

func readAllBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(r.Body)
	return buf.Bytes(), err
}

func TestClientAssertionClaimsAndSignature(t *testing.T) {
	keyPEM, pub := testKeyPEM(t)
	c := credentials{Username: "rep@acme.com", ClientID: "cid-123", PrivateKey: keyPEM}
	assertion, err := buildClientAssertion(c, mustNow())
	if err != nil {
		t.Fatalf("buildClientAssertion: %v", err)
	}
	claims := verifyAssertion(t, assertion, pub)
	if claims["aud"] != jwtAudience {
		t.Errorf("aud = %v, want %q", claims["aud"], jwtAudience)
	}
	if claims["iss"] != jwtIssuer {
		t.Errorf("iss = %v, want %q", claims["iss"], jwtIssuer)
	}
	if claims["client_id"] != "cid-123" {
		t.Errorf("client_id = %v", claims["client_id"])
	}
	if claims["username"] != "rep@acme.com" {
		t.Errorf("username = %v", claims["username"])
	}
	iat, iatOK := claims["iat"].(float64)
	exp, expOK := claims["exp"].(float64)
	if !iatOK || !expOK {
		t.Fatalf("iat/exp missing: %v", claims)
	}
	if int(exp-iat) != int(clientAssertionTTL.Seconds()) {
		t.Errorf("exp-iat = %d, want %d", int(exp-iat), int(clientAssertionTTL.Seconds()))
	}
}

func TestContactEnrichExchangesAndCallsDataEndpoint(t *testing.T) {
	keyPEM, pub := testKeyPEM(t)
	fake := &fakeServer{pub: pub, accessJWT: "access-jwt-xyz", dataResponse: `{"data":[{"id":1}],"creditsConsumed":1}`}
	srv := httptest.NewServer(fake.handler(t))
	defer srv.Close()

	var out, errOut bytes.Buffer
	svc := &Service{BaseURL: srv.URL, Out: &out, Err: &errOut}
	res, err := svc.Execute(context.Background(),
		[]string{"contact", "enrich", "--body", `{"matchPersonInput":[{"personId":1}],"outputFields":["email"]}`},
		credEnv(t, keyPEM))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%s", res.ExitCode, errOut.String())
	}
	if fake.assertion["client_id"] != "cid-123" {
		t.Errorf("authenticate never saw the client assertion: %v", fake.assertion)
	}
	if fake.lastAuthHdr != "Bearer access-jwt-xyz" {
		t.Errorf("data Authorization = %q, want Bearer access-jwt-xyz", fake.lastAuthHdr)
	}
	if fake.lastPath != "/enrich/contact" || fake.lastMethod != http.MethodPost {
		t.Errorf("data call = %s %s, want POST /enrich/contact", fake.lastMethod, fake.lastPath)
	}
	if !strings.Contains(string(fake.lastBody), "matchPersonInput") {
		t.Errorf("request body not forwarded: %s", fake.lastBody)
	}
	if !strings.Contains(out.String(), "creditsConsumed") {
		t.Errorf("response not emitted: %s", out.String())
	}
}

func TestCompanySearchAndLookupAndUsagePaths(t *testing.T) {
	keyPEM, pub := testKeyPEM(t)
	cases := []struct {
		args   []string
		method string
		path   string
	}{
		{[]string{"company", "search", "--body", `{"companyName":"Acme"}`}, http.MethodPost, "/search/company"},
		{[]string{"contact", "search", "--body", `{"jobTitle":"CTO"}`}, http.MethodPost, "/search/contact"},
		{[]string{"company", "enrich", "--body", `{"matchCompanyInput":[{"companyId":9}]}`}, http.MethodPost, "/enrich/company"},
		{[]string{"lookup", "inputFields/contact"}, http.MethodGet, "/lookup/inputFields/contact"},
		{[]string{"usage"}, http.MethodGet, "/usage"},
	}
	for _, tc := range cases {
		fake := &fakeServer{pub: pub, accessJWT: "at", dataResponse: `{"ok":true}`}
		srv := httptest.NewServer(fake.handler(t))
		var out, errOut bytes.Buffer
		svc := &Service{BaseURL: srv.URL, Out: &out, Err: &errOut}
		res, err := svc.Execute(context.Background(), tc.args, credEnv(t, keyPEM))
		if err != nil {
			t.Fatalf("%v: Execute err %v", tc.args, err)
		}
		if res.ExitCode != 0 {
			t.Fatalf("%v: exit %d stderr=%s", tc.args, res.ExitCode, errOut.String())
		}
		if fake.lastMethod != tc.method || fake.lastPath != tc.path {
			t.Errorf("%v: got %s %s, want %s %s", tc.args, fake.lastMethod, fake.lastPath, tc.method, tc.path)
		}
		srv.Close()
	}
}

func TestUnauthorizedClassifiesCredentialRejection(t *testing.T) {
	keyPEM, pub := testKeyPEM(t)
	fake := &fakeServer{pub: pub, accessJWT: "at", dataStatus: http.StatusUnauthorized, dataResponse: `{"error":"expired"}`}
	srv := httptest.NewServer(fake.handler(t))
	defer srv.Close()

	var out, errOut bytes.Buffer
	svc := &Service{BaseURL: srv.URL, Out: &out, Err: &errOut}
	res, err := svc.Execute(context.Background(), []string{"usage", "--json"}, credEnv(t, keyPEM))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.ExitCode != 1 || !res.CredentialRejected {
		t.Fatalf("res = %+v, want exit 1 + credential rejection", res)
	}
	if !strings.Contains(errOut.String(), `"kind":"api"`) || !strings.Contains(errOut.String(), `"status":401`) {
		t.Errorf("json error envelope missing api/status: %s", errOut.String())
	}
}

func TestMissingCredentialIsUsageExit(t *testing.T) {
	var out, errOut bytes.Buffer
	svc := &Service{Out: &out, Err: &errOut}
	res, err := svc.Execute(context.Background(), []string{"usage", "--json"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2", res.ExitCode)
	}
	if !strings.Contains(errOut.String(), "ZOOMINFO_CREDENTIALS") {
		t.Errorf("stderr should name the missing env: %s", errOut.String())
	}
}

func TestInvalidBodyIsUsageExitBeforeNetwork(t *testing.T) {
	keyPEM, pub := testKeyPEM(t)
	fake := &fakeServer{pub: pub, accessJWT: "at"}
	srv := httptest.NewServer(fake.handler(t))
	defer srv.Close()

	var out, errOut bytes.Buffer
	svc := &Service{BaseURL: srv.URL, Out: &out, Err: &errOut}
	res, err := svc.Execute(context.Background(), []string{"contact", "search", "--body", `{not json`}, credEnv(t, keyPEM))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", res.ExitCode)
	}
	if fake.lastPath != "" {
		t.Errorf("network was hit for an invalid body: %s", fake.lastPath)
	}
}

func TestUnknownSubcommandIsUsageExit(t *testing.T) {
	keyPEM, _ := testKeyPEM(t)
	var out, errOut bytes.Buffer
	svc := &Service{Out: &out, Err: &errOut}
	res, err := svc.Execute(context.Background(), []string{"contact", "bogus"}, credEnv(t, keyPEM))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2", res.ExitCode)
	}
}

func TestPKCS1PrivateKeyIsAccepted(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pkcs1 := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	if _, err := buildClientAssertion(credentials{Username: "u", ClientID: "c", PrivateKey: pkcs1}, mustNow()); err != nil {
		t.Fatalf("PKCS#1 key rejected: %v", err)
	}
}
