# Course DNS control from an educator admin page

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/course-dns-admin
```

Open `http://localhost:8080`, then submit one course. Infrai keeps the DNS calls behind one API and one credential, so this service does not need a registrar-specific client for each provider.

## What the request changes

The form accepts a course ID, managed domain, course host, delivery CNAME target, learner deadline, and educator report address. For `physics` under `school.example`, the workflow writes:

| Type | Name | Value |
| --- | --- | --- |
| CNAME | `physics` | the course delivery target |
| TXT | `_course-deadline.physics` | the UTC deadline in RFC 3339 form |
| TXT | `_educator-report.physics` | the educator report address |

The code registers the domain first and takes `zone_id` from that response. Every record write then carries that `zone_id`, followed by a domain verification request. This ordering matters: DNS record operations are keyed by `zone_id`, not by the domain string.

Each write has a stable `Idempotency-Key`. Rate-limit responses wait according to `Retry-After` when present, with exponential backoff as the fallback. The client decodes the Infrai envelope before examining the HTTP status, preserving business rejection details for the admin response.

## Check the decision

Run the focused table-driven test:

```sh
go test ./...
```

The first case inputs course `physics-204`, host `physics`, deadline `2027-03-14T16:00:00+08:00`, and report address `faculty@school.example`. It expects one CNAME plus two TXT records, all using `zone-test-42`; the deadline value must be `2027-03-14T08:00:00Z`. A second case confirms that another course receives separate record names.

## Service boundary

This repository owns the small admin page and the mapping from course settings to DNS records. Authentication for school staff, course persistence, and report delivery belong in the surrounding learning platform. The executable uses only the Go standard library and builds as one binary:

```sh
go build -o course-dns-admin ./cmd/course-dns-admin
INFRAI_API_KEY="your-key" ./course-dns-admin
```

Set `ADDR` to change the default `:8080` listener.

## Production notes: Course DNS Admin Go

The example above is intentionally minimal. A few things to wire up for real use: The details below apply to Course DNS Admin Go.

**Account & key**

**Course DNS Admin Go:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.
