# ADR 0001: Independent DNS and management foundation

Status: accepted.

The DNS pipeline is implemented in Go. miekg/dns provides wire parsing and transports;
Velora owns access control, forwarding, retries, caching, and later zone/filter policy.
No external DNS server process is required. Cache is bounded, in-memory, and independent
of SQL. SQLite is behind the management storage boundary; future PostgreSQL adapters
must not enter the hot cache path. HTTP handlers depend on narrow operational interfaces.

Default listeners and client CIDRs are loopback-only. Docker binds container sockets to
all interfaces but publishes only to host loopback. There is no iterative recursion or
DNSSEC validation in the foundation. Unsupported management features have no fake API.

The web build is served by the Go HTTP server in production. Development uses Vite with
an API proxy. Application shutdown cancels resolver work and drains both HTTP and DNS.
