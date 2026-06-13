// Package profiles defines custom tls-client ClientProfiles. Chrome_147 is a
// Chrome 147 ClientHello profile for Windows, with the pre_shared_key extension
// placed last so utls emits it only on resumed sessions.
//
// Extension structs are constructed fresh inside SpecFactory on every handshake
// because utls mutates KeyShare and PSK state during the handshake; sharing them
// across requests triggers "tls: internal error: failed to update binders".
package profiles

import (
	"github.com/bogdanfinn/fhttp/http2"
	tls_client "github.com/bogdanfinn/tls-client/profiles"
	tls "github.com/bogdanfinn/utls"
)

var Chrome_147 = tls_client.NewClientProfile(
	tls.ClientHelloID{
		Client:               "Chrome",
		RandomExtensionOrder: false,
		Version:              "147",
		Seed:                 nil,
		SpecFactory: func() (tls.ClientHelloSpec, error) {
			return tls.ClientHelloSpec{
				CipherSuites: []uint16{
					tls.GREASE_PLACEHOLDER,
					tls.TLS_AES_128_GCM_SHA256,
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				},
				CompressionMethods: []byte{tls.CompressionNone},
				Extensions: []tls.TLSExtension{
					&tls.UtlsGREASEExtension{},
					&tls.ApplicationSettingsExtensionNew{SupportedProtocols: []string{"h2"}},
					&tls.SupportedVersionsExtension{Versions: []uint16{
						tls.GREASE_PLACEHOLDER,
						tls.VersionTLS13,
						tls.VersionTLS12,
					}},
					&tls.SCTExtension{},
					tls.BoringGREASEECH(),
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
						{Group: tls.X25519MLKEM768},
						{Group: tls.X25519},
					}},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.ECDSAWithP256AndSHA256,
						tls.PSSWithSHA256,
						tls.PKCS1WithSHA256,
						tls.ECDSAWithP384AndSHA384,
						tls.PSSWithSHA384,
						tls.PKCS1WithSHA384,
						tls.PSSWithSHA512,
						tls.PKCS1WithSHA512,
					}},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{
						tls.GREASE_PLACEHOLDER,
						tls.X25519MLKEM768,
						tls.X25519,
						tls.CurveP256,
						tls.CurveP384,
					}},
					&tls.UtlsCompressCertExtension{Algorithms: []tls.CertCompressionAlgo{tls.CertCompressionBrotli}},
					&tls.ExtendedMasterSecretExtension{},
					&tls.SessionTicketExtension{},
					&tls.SNIExtension{},
					&tls.RenegotiationInfoExtension{Renegotiation: tls.RenegotiateOnceAsClient},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{tls.PskModeDHE}},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{tls.PointFormatUncompressed}},
					&tls.StatusRequestExtension{},
					&tls.ALPNExtension{AlpnProtocols: []string{"h2", "http/1.1"}},
					&tls.UtlsGREASEExtension{},
					&tls.UtlsPreSharedKeyExtension{OmitEmptyPsk: true},
				},
			}, nil
		},
	},
	map[http2.SettingID]uint32{
		http2.SettingHeaderTableSize:   65536,
		http2.SettingEnablePush:        0,
		http2.SettingInitialWindowSize: 6291456,
		http2.SettingMaxHeaderListSize: 262144,
	},
	[]http2.SettingID{
		http2.SettingHeaderTableSize,
		http2.SettingEnablePush,
		http2.SettingInitialWindowSize,
		http2.SettingMaxHeaderListSize,
	},
	[]string{":method", ":authority", ":scheme", ":path"},
	15663105,
	nil,
	nil,
	0,
	false,
	nil,
	nil,
	0,
	nil,
	false,
)
