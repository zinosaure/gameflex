# FreeROMs intermediate chain

Public issuer certificates obtained from the certificate AIA URLs on 2026-11-08:

- `http://ye2.i.lencr.org/` — Let's Encrypt YE2, issued by ISRG Root YE.
- `http://ye.i.lencr.org/` — Root YE cross-signed by ISRG Root X2.

The cross-signed certificate verified against the existing system CA trust store
(`openssl verify -CApath /etc/ssl/certs`). The complete connection then verified
with hostname checking and the existing ISRG Root X2 trust anchor.

These are intermediate certificates, not private keys or additional trust roots.
The provider SSL context requires a complete chain to a system-trusted root and
explicitly disables partial-chain trust. No system certificate store is modified.
