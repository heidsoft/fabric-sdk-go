/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"testing"
	"time"

	"fmt"
	"strings"

	"github.com/golang/mock/gomock"
	fabApi "github.com/hyperledger/fabric-sdk-go/pkg/common/providers/fab"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/providers/msp"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/providers/test/mockcontext"
	mockmspApi "github.com/hyperledger/fabric-sdk-go/pkg/common/providers/test/mockmsp"
	"github.com/hyperledger/fabric-sdk-go/pkg/core/config"
	"github.com/hyperledger/fabric-sdk-go/pkg/core/config/endpoint"
	"github.com/hyperledger/fabric-sdk-go/pkg/core/config/lookup"
	bccspwrapper "github.com/hyperledger/fabric-sdk-go/pkg/core/cryptosuite/bccsp/wrapper"
	"github.com/hyperledger/fabric-sdk-go/pkg/core/mocks"
	"github.com/hyperledger/fabric-sdk-go/pkg/fab"
	"github.com/hyperledger/fabric-sdk-go/pkg/msp/api"
	"github.com/hyperledger/fabric-sdk-go/pkg/msp/test/mockmsp"
	"github.com/pkg/errors"
)

// TestEnrollAndReenroll tests enrol/reenroll scenarios
func TestEnrollAndReenroll(t *testing.T) {

	f := textFixture{}
	f.setup(nil)
	defer f.close()

	orgMSPID := mspIDByOrgName(t, f.endpointConfig, org1)

	// Empty enrollment ID
	err := f.caClient.Enroll("", "user1")
	if err == nil {
		t.Fatalf("Enroll didn't return error")
	}

	// Empty enrollment secret
	err = f.caClient.Enroll("enrolledUsername", "")
	if err == nil {
		t.Fatalf("Enroll didn't return error")
	}

	// Successful enrollment
	enrollUsername := createRandomName()
	_, err = f.userStore.Load(msp.IdentityIdentifier{MSPID: orgMSPID, ID: enrollUsername})
	if err != msp.ErrUserNotFound {
		t.Fatalf("Expected to not find user in user store")
	}
	err = f.caClient.Enroll(enrollUsername, "enrollmentSecret")
	if err != nil {
		t.Fatalf("identityManager Enroll return error %v", err)
	}
	enrolledUserData, err := f.userStore.Load(msp.IdentityIdentifier{MSPID: orgMSPID, ID: enrollUsername})
	if err != nil {
		t.Fatalf("Expected to load user from user store")
	}

	// Reenroll with empty user
	err = f.caClient.Reenroll("")
	if err == nil {
		t.Fatalf("Expected error with enpty user")
	}
	if err.Error() != "user name missing" {
		t.Fatalf("Expected error user required. Got: %s", err.Error())
	}

	// Reenroll with appropriate user
	reenrollWithAppropriateUser(f, t, enrolledUserData)
}

func reenrollWithAppropriateUser(f textFixture, t *testing.T, enrolledUserData *msp.UserData) {
	iManager, ok := f.identityManagerProvider.IdentityManager("org1")
	if !ok {
		t.Fatalf("failed to get identity manager")
	}
	enrolledUser, err := iManager.(*IdentityManager).NewUser(enrolledUserData)
	if err != nil {
		t.Fatalf("newUser return error %v", err)
	}
	err = f.caClient.Reenroll(enrolledUser.Identifier().ID)
	if err != nil {
		t.Fatalf("Reenroll return error %v", err)
	}
}

// TestWrongURL tests creation of CAClient with wrong URL
func TestWrongURL(t *testing.T) {

	f := textFixture{}
	f.setup(nil)
	defer f.close()

	configBackend, err := getInvalidURLBackend()
	if err != nil {
		panic(fmt.Sprintf("Failed to get config backend: %v", err))
	}

	wrongURLIdentityConfig, err := ConfigFromBackend(configBackend)
	if err != nil {
		panic(fmt.Sprintf("Failed to read config: %v", err))
	}

	wrongURLEndpointConfig, err := fab.ConfigFromBackend(configBackend)
	if err != nil {
		panic(fmt.Sprintf("Failed to read config: %v", err))
	}

	iManager, ok := f.identityManagerProvider.IdentityManager("Org1")
	if !ok {
		t.Fatalf("failed to get identity manager")
	}

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockContext := mockcontext.NewMockClient(mockCtrl)
	mockContext.EXPECT().EndpointConfig().Return(wrongURLEndpointConfig).AnyTimes()
	mockContext.EXPECT().IdentityConfig().Return(wrongURLIdentityConfig).AnyTimes()
	mockContext.EXPECT().CryptoSuite().Return(f.cryptoSuite).AnyTimes()
	mockContext.EXPECT().UserStore().Return(f.userStore).AnyTimes()
	mockContext.EXPECT().IdentityManager("Org1").Return(iManager, true).AnyTimes()

	//f.caClient, err = NewCAClient(org1, f.identityManager, f.userStore, f.cryptoSuite, wrongURLConfigConfig)
	f.caClient, err = NewCAClient(org1, mockContext)
	if err != nil {
		t.Fatalf("NewidentityManagerClient return error: %v", err)
	}
	err = f.caClient.Enroll("enrollmentID", "enrollmentSecret")
	if err == nil {
		t.Fatalf("Enroll didn't return error")
	}

}

// TestNoConfiguredCAs tests creation of CAClient when there are no configured CAs
func TestNoConfiguredCAs(t *testing.T) {

	f := textFixture{}
	f.setup(nil)
	defer f.close()

	configBackend, err := getNoCAConfigBackend()
	if err != nil {
		panic(fmt.Sprintf("Failed to get config backend: %v", err))
	}

	wrongURLEndpointConfig, err := fab.ConfigFromBackend(configBackend)
	if err != nil {
		panic(fmt.Sprintf("Failed to read config: %v", err))
	}

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockContext := mockcontext.NewMockClient(mockCtrl)
	mockContext.EXPECT().EndpointConfig().Return(wrongURLEndpointConfig).AnyTimes()
	mockContext.EXPECT().IdentityConfig().Return(f.identityConfig).AnyTimes()
	mockContext.EXPECT().CryptoSuite().Return(f.cryptoSuite).AnyTimes()
	mockContext.EXPECT().UserStore().Return(f.userStore).AnyTimes()

	_, err = NewCAClient(org1, mockContext)
	if err == nil || !strings.Contains(err.Error(), "no CAs configured") {
		t.Fatalf("Expected error when there are no configured CAs")
	}

}

// TestRegister tests multiple scenarios of registering a test (mocked or nil user) and their certs
func TestRegister(t *testing.T) {

	time.Sleep(2 * time.Second)

	f := textFixture{}
	f.setup(nil)
	defer f.close()

	// Register with nil request
	_, err := f.caClient.Register(nil)
	if err == nil {
		t.Fatalf("Expected error with nil request")
	}

	// Register without registration name parameter
	_, err = f.caClient.Register(&api.RegistrationRequest{})
	if err == nil {
		t.Fatalf("Expected error without registration name parameter")
	}

	// Register with valid request
	var attributes []api.Attribute
	attributes = append(attributes, api.Attribute{Name: "test1", Value: "test2"})
	attributes = append(attributes, api.Attribute{Name: "test2", Value: "test3"})
	secret, err := f.caClient.Register(&api.RegistrationRequest{Name: "test", Affiliation: "test", Attributes: attributes})
	if err != nil {
		t.Fatalf("identityManager Register return error %v", err)
	}
	if secret != "mockSecretValue" {
		t.Fatalf("identityManager Register return wrong value %s", secret)
	}
}

// TestEmbeddedRegistar tests registration with embedded registrar identity
func TestEmbeddedRegistar(t *testing.T) {

	embeddedRegistrarBackend, err := getEmbeddedRegistrarConfigBackend()
	if err != nil {
		t.Fatalf("Failed to get config backend, cause: %v", err)
	}

	f := textFixture{}
	f.setup(embeddedRegistrarBackend)
	defer f.close()

	// Register with valid request
	var attributes []api.Attribute
	attributes = append(attributes, api.Attribute{Name: "test1", Value: "test2"})
	attributes = append(attributes, api.Attribute{Name: "test2", Value: "test3"})
	secret, err := f.caClient.Register(&api.RegistrationRequest{Name: "withEmbeddedRegistrar", Affiliation: "test", Attributes: attributes})
	if err != nil {
		t.Fatalf("identityManager Register return error %v", err)
	}
	if secret != "mockSecretValue" {
		t.Fatalf("identityManager Register return wrong value %s", secret)
	}
}

// TestRegisterNoRegistrar tests registration with no configured registrar identity
func TestRegisterNoRegistrar(t *testing.T) {

	noRegistrarBackend, err := getNoRegistrarBackend()
	if err != nil {
		t.Fatalf("Failed to get config backend, cause: %v", err)
	}

	f := textFixture{}
	f.setup(noRegistrarBackend)
	defer f.close()

	// Register with nil request
	_, err = f.caClient.Register(nil)
	if err != api.ErrCARegistrarNotFound {
		t.Fatalf("Expected ErrCARegistrarNotFound, got: %v", err)
	}

	// Register without registration name parameter
	_, err = f.caClient.Register(&api.RegistrationRequest{})
	if err != api.ErrCARegistrarNotFound {
		t.Fatalf("Expected ErrCARegistrarNotFound, got: %v", err)
	}

	// Register with valid request
	var attributes []api.Attribute
	attributes = append(attributes, api.Attribute{Name: "test1", Value: "test2"})
	attributes = append(attributes, api.Attribute{Name: "test2", Value: "test3"})
	_, err = f.caClient.Register(&api.RegistrationRequest{Name: "test", Affiliation: "test", Attributes: attributes})
	if err != api.ErrCARegistrarNotFound {
		t.Fatalf("Expected ErrCARegistrarNotFound, got: %v", err)
	}
}

// TestRevoke will test multiple revoking a user with a nil request or a nil user
// TODO - improve Revoke test coverage
func TestRevoke(t *testing.T) {

	f := textFixture{}
	f.setup(nil)
	defer f.close()

	// Revoke with nil request
	_, err := f.caClient.Revoke(nil)
	if err == nil {
		t.Fatalf("Expected error with nil request")
	}

	mockKey := bccspwrapper.GetKey(&mockmsp.MockKey{})
	user := mockmsp.NewMockSigningIdentity("test", "test")
	user.SetEnrollmentCertificate(readCert(t))
	user.SetPrivateKey(mockKey)

	_, err = f.caClient.Revoke(&api.RevocationRequest{})
	if err == nil {
		t.Fatalf("Expected decoding error with test cert")
	}
}

// TestCAConfigError will test CAClient creation with bad CAConfig
func TestCAConfigError(t *testing.T) {

	f := textFixture{}
	f.setup(nil)
	defer f.close()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockContext := mockcontext.NewMockClient(mockCtrl)

	mockIdentityConfig := mockmspApi.NewMockIdentityConfig(mockCtrl)
	mockIdentityConfig.EXPECT().CAConfig(org1).Return(nil, errors.New("CAConfig error"))
	mockIdentityConfig.EXPECT().CredentialStorePath().Return(dummyUserStorePath).AnyTimes()

	mockContext.EXPECT().IdentityConfig().Return(mockIdentityConfig)
	mockContext.EXPECT().EndpointConfig().Return(f.endpointConfig).AnyTimes()

	_, err := NewCAClient(org1, mockContext)
	if err == nil || !strings.Contains(err.Error(), "CAConfig error") {
		t.Fatalf("Expected error from CAConfig. Got: %v", err)
	}
}

// TestCAServerCertPathsError will test CAClient creation with missing CAServerCertPaths
func TestCAServerCertPathsError(t *testing.T) {

	f := textFixture{}
	f.setup(nil)
	defer f.close()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockIdentityConfig := mockmspApi.NewMockIdentityConfig(mockCtrl)
	mockIdentityConfig.EXPECT().CAConfig(org1).Return(&msp.CAConfig{}, nil).AnyTimes()
	mockIdentityConfig.EXPECT().CredentialStorePath().Return(dummyUserStorePath).AnyTimes()
	mockIdentityConfig.EXPECT().CAServerCerts(org1).Return(nil, errors.New("CAServerCerts error"))

	mockContext := mockcontext.NewMockClient(mockCtrl)
	mockContext.EXPECT().EndpointConfig().Return(f.endpointConfig).AnyTimes()
	mockContext.EXPECT().IdentityConfig().Return(mockIdentityConfig).AnyTimes()
	mockContext.EXPECT().UserStore().Return(&mockmsp.MockUserStore{}).AnyTimes()
	mockContext.EXPECT().CryptoSuite().Return(f.cryptoSuite).AnyTimes()

	_, err := NewCAClient(org1, mockContext)
	if err == nil || !strings.Contains(err.Error(), "CAServerCerts error") {
		t.Fatalf("Expected error from CAServerCertPaths. Got: %v", err)
	}
}

// TestCAClientCertPathError will test CAClient creation with missing CAClientCertPath
func TestCAClientCertPathError(t *testing.T) {

	f := textFixture{}
	f.setup(nil)
	defer f.close()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockIdentityConfig := mockmspApi.NewMockIdentityConfig(mockCtrl)
	mockIdentityConfig.EXPECT().CAConfig(org1).Return(&msp.CAConfig{}, nil).AnyTimes()
	mockIdentityConfig.EXPECT().CredentialStorePath().Return(dummyUserStorePath).AnyTimes()
	mockIdentityConfig.EXPECT().CAServerCerts(org1).Return([][]byte{[]byte("test")}, nil)
	mockIdentityConfig.EXPECT().CAClientCert(org1).Return(nil, errors.New("CAClientCertPath error"))

	mockContext := mockcontext.NewMockClient(mockCtrl)
	mockContext.EXPECT().EndpointConfig().Return(f.endpointConfig).AnyTimes()
	mockContext.EXPECT().IdentityConfig().Return(mockIdentityConfig).AnyTimes()
	mockContext.EXPECT().UserStore().Return(&mockmsp.MockUserStore{}).AnyTimes()
	mockContext.EXPECT().CryptoSuite().Return(f.cryptoSuite).AnyTimes()

	_, err := NewCAClient(org1, mockContext)
	if err == nil || !strings.Contains(err.Error(), "CAClientCertPath error") {
		t.Fatalf("Expected error from CAClientCertPath. Got: %v", err)
	}
}

// TestCAClientKeyPathError will test CAClient creation with missing CAClientKeyPath
func TestCAClientKeyPathError(t *testing.T) {

	f := textFixture{}
	f.setup(nil)
	defer f.close()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockIdentityConfig := mockmspApi.NewMockIdentityConfig(mockCtrl)
	mockIdentityConfig.EXPECT().CAConfig(org1).Return(&msp.CAConfig{}, nil).AnyTimes()
	mockIdentityConfig.EXPECT().CredentialStorePath().Return(dummyUserStorePath).AnyTimes()
	mockIdentityConfig.EXPECT().CAServerCerts(org1).Return([][]byte{[]byte("test")}, nil)
	mockIdentityConfig.EXPECT().CAClientCert(org1).Return([]byte(""), nil)
	mockIdentityConfig.EXPECT().CAClientKey(org1).Return(nil, errors.New("CAClientKeyPath error"))

	mockContext := mockcontext.NewMockClient(mockCtrl)
	mockContext.EXPECT().EndpointConfig().Return(f.endpointConfig).AnyTimes()
	mockContext.EXPECT().IdentityConfig().Return(mockIdentityConfig).AnyTimes()
	mockContext.EXPECT().UserStore().Return(&mockmsp.MockUserStore{}).AnyTimes()
	mockContext.EXPECT().CryptoSuite().Return(f.cryptoSuite).AnyTimes()

	_, err := NewCAClient(org1, mockContext)
	if err == nil || !strings.Contains(err.Error(), "CAClientKeyPath error") {
		t.Fatalf("Expected error from CAClientKeyPath. Got: %v", err)
	}
}

// TestInterfaces will test if the interface instantiation happens properly, ie no nil returned
func TestInterfaces(t *testing.T) {
	var apiClient api.CAClient
	var cl CAClientImpl

	apiClient = &cl
	if apiClient == nil {
		t.Fatalf("this shouldn't happen.")
	}
}

func getCustomBackend(configPath string) (*mocks.MockConfigBackend, error) {

	backend, err := config.FromFile(configPath)()
	if err != nil {
		return nil, err
	}
	backendMap := make(map[string]interface{})
	backendMap["client"], _ = backend.Lookup("client")
	backendMap["certificateAuthorities"], _ = backend.Lookup("certificateAuthorities")
	backendMap["entityMatchers"], _ = backend.Lookup("entityMatchers")
	backendMap["peers"], _ = backend.Lookup("peers")
	backendMap["organizations"], _ = backend.Lookup("organizations")
	backendMap["orderers"], _ = backend.Lookup("orderers")
	backendMap["channels"], _ = backend.Lookup("channels")

	return &mocks.MockConfigBackend{KeyValueMap: backendMap, CustomBackend: backend}, nil
}

func getInvalidURLBackend() (*mocks.MockConfigBackend, error) {

	mockConfigBackend, err := getCustomBackend(configPath)
	if err != nil {
		return nil, err
	}

	//Create an invalid channel
	networkConfig := fabApi.NetworkConfig{}
	//get valid certificate authorities
	err = lookup.New(mockConfigBackend).UnmarshalKey("certificateAuthorities", &networkConfig.CertificateAuthorities)
	if err != nil {
		return nil, err
	}

	//tamper URLs
	ca1Config := networkConfig.CertificateAuthorities["ca.org1.example.com"]
	ca1Config.URL = "http://localhost:8091"
	ca2Config := networkConfig.CertificateAuthorities["ca.org2.example.com"]
	ca2Config.URL = "http://localhost:8091"

	networkConfig.CertificateAuthorities["ca.org1.example.com"] = ca1Config
	networkConfig.CertificateAuthorities["ca.org2.example.com"] = ca2Config

	//Override backend with this new CertificateAuthorities config
	mockConfigBackend.KeyValueMap["certificateAuthorities"] = networkConfig.CertificateAuthorities

	return mockConfigBackend, nil
}

func getNoRegistrarBackend() (*mocks.MockConfigBackend, error) {

	mockConfigBackend, err := getCustomBackend(configPath)
	if err != nil {
		return nil, err
	}

	//Create an invalid channel
	networkConfig := fabApi.NetworkConfig{}
	//get valid certificate authorities
	err = lookup.New(mockConfigBackend).UnmarshalKey("certificateAuthorities", &networkConfig.CertificateAuthorities)
	if err != nil {
		return nil, err
	}

	//tamper URLs
	ca1Config := networkConfig.CertificateAuthorities["ca.org1.example.com"]
	ca1Config.Registrar = msp.EnrollCredentials{}
	ca2Config := networkConfig.CertificateAuthorities["ca.org2.example.com"]
	ca1Config.Registrar = msp.EnrollCredentials{}

	networkConfig.CertificateAuthorities["ca.org1.example.com"] = ca1Config
	networkConfig.CertificateAuthorities["ca.org2.example.com"] = ca2Config

	//Override backend with this new CertificateAuthorities config
	mockConfigBackend.KeyValueMap["certificateAuthorities"] = networkConfig.CertificateAuthorities

	return mockConfigBackend, nil
}

func getNoCAConfigBackend() (*mocks.MockConfigBackend, error) {

	mockConfigBackend, err := getCustomBackend(configPath)
	if err != nil {
		return nil, err
	}

	//Create an empty network config
	networkConfig := fabApi.NetworkConfig{}
	//get valid certificate authorities
	err = lookup.New(mockConfigBackend).UnmarshalKey("organizations", &networkConfig.Organizations)
	if err != nil {
		return nil, err
	}
	org1 := networkConfig.Organizations["org1"]

	//clear certificate authorities
	org1.CertificateAuthorities = []string{}
	networkConfig.Organizations["org1"] = org1

	//Override backend with organization config having empty CertificateAuthorities
	mockConfigBackend.KeyValueMap["organizations"] = networkConfig.Organizations
	//Override backend with this nil empty CertificateAuthorities config
	mockConfigBackend.KeyValueMap["certificateAuthorities"] = networkConfig.CertificateAuthorities

	return mockConfigBackend, nil
}

func getEmbeddedRegistrarConfigBackend() (*mocks.MockConfigBackend, error) {

	mockConfigBackend, err := getCustomBackend(configPath)
	if err != nil {
		return nil, err
	}

	embeddedRegistrarID := "embeddedregistrar"

	//Create an empty network config
	networkConfig := fabApi.NetworkConfig{}
	//get valid certificate authorities
	err = lookup.New(mockConfigBackend).UnmarshalKey("organizations", &networkConfig.Organizations)
	if err != nil {
		return nil, err
	}
	err = lookup.New(mockConfigBackend).UnmarshalKey("certificateAuthorities", &networkConfig.CertificateAuthorities)
	if err != nil {
		return nil, err
	}
	//update with embedded registrar
	org1 := networkConfig.Organizations["org1"]
	org1.Users = make(map[string]endpoint.TLSKeyPair)
	org1.Users[embeddedRegistrarID] = endpoint.TLSKeyPair{
		Key: endpoint.TLSConfig{
			Pem: `-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgp4qKKB0WCEfx7XiB
5Ul+GpjM1P5rqc6RhjD5OkTgl5OhRANCAATyFT0voXX7cA4PPtNstWleaTpwjvbS
J3+tMGTG67f+TdCfDxWYMpQYxLlE8VkbEzKWDwCYvDZRMKCQfv2ErNvb
-----END PRIVATE KEY-----`,
		},
		Cert: endpoint.TLSConfig{
			Pem: `-----BEGIN CERTIFICATE-----
MIICGTCCAcCgAwIBAgIRALR/1GXtEud5GQL2CZykkOkwCgYIKoZIzj0EAwIwczEL
MAkGA1UEBhMCVVMxEzARBgNVBAgTCkNhbGlmb3JuaWExFjAUBgNVBAcTDVNhbiBG
cmFuY2lzY28xGTAXBgNVBAoTEG9yZzEuZXhhbXBsZS5jb20xHDAaBgNVBAMTE2Nh
Lm9yZzEuZXhhbXBsZS5jb20wHhcNMTcwNzI4MTQyNzIwWhcNMjcwNzI2MTQyNzIw
WjBbMQswCQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZvcm5pYTEWMBQGA1UEBxMN
U2FuIEZyYW5jaXNjbzEfMB0GA1UEAwwWVXNlcjFAb3JnMS5leGFtcGxlLmNvbTBZ
MBMGByqGSM49AgEGCCqGSM49AwEHA0IABPIVPS+hdftwDg8+02y1aV5pOnCO9tIn
f60wZMbrt/5N0J8PFZgylBjEuUTxWRsTMpYPAJi8NlEwoJB+/YSs29ujTTBLMA4G
A1UdDwEB/wQEAwIHgDAMBgNVHRMBAf8EAjAAMCsGA1UdIwQkMCKAIIeR0TY+iVFf
mvoEKwaToscEu43ZXSj5fTVJornjxDUtMAoGCCqGSM49BAMCA0cAMEQCID+dZ7H5
AiaiI2BjxnL3/TetJ8iFJYZyWvK//an13WV/AiARBJd/pI5A7KZgQxJhXmmR8bie
XdsmTcdRvJ3TS/6HCA==
-----END CERTIFICATE-----`,
		},
	}
	networkConfig.Organizations["org1"] = org1

	//update network certificate authorities
	ca1Config := networkConfig.CertificateAuthorities["ca.org1.example.com"]
	ca1Config.Registrar = msp.EnrollCredentials{EnrollID: embeddedRegistrarID}
	ca2Config := networkConfig.CertificateAuthorities["ca.org2.example.com"]
	ca2Config.Registrar = msp.EnrollCredentials{EnrollID: embeddedRegistrarID}
	networkConfig.CertificateAuthorities["ca.org1.example.com"] = ca1Config
	networkConfig.CertificateAuthorities["ca.org2.example.com"] = ca2Config

	//Override backend with updated organization config
	mockConfigBackend.KeyValueMap["organizations"] = networkConfig.Organizations
	//Override backend with updated certificate authorities config
	mockConfigBackend.KeyValueMap["certificateAuthorities"] = networkConfig.CertificateAuthorities

	return mockConfigBackend, nil
}
