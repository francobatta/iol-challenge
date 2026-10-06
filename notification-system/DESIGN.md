## notification system

Alex Xu's problem statement and solution omit natural complexities that arise when designing a notification system. How do you...

* support multiple clients with different requirements?
* get from a notification request to in-flight worker messages?
* handle rate limiting in a semi real world setting?
* distribute the workers and use policies to avoid starvation of different clients/jobs/workloads?
* use an efficient and durable data structure for messages?

This application tackles the above by:

* supporting multitenancy enabling 'apps' (app === client of our system)
* scaling workers across different apps and 'providers' (twilio, email, android && apple push messaging are providers):
  - KEDA scaler based on queue fill rate and overall length
  - one worker per provider, with app fairness and limited buffer lengths (prefetch)
  - one queue per app per provider so messages are clearly segmented (other solutions possible such as N partitions by hashing app ID, etc.)
* having defensive mechanisms to ensure correctness and avoid overwhelming any part of the system, via:
  - circuit breakers at workers (not distributed, very well could be using redis counters)
  - quorum queues data structure which enables scaling (not FIFO, high priority and normal priority jobs)
  - 'daily quota' for each app, to ensure potential paying tiers are enforced, preventing abuse
  - deadletter queue

### The gist of the system
The 'api' abstraction exposes a set of endpoints to register new users and its endpoints (where the notification is actually sent)

The 'api' receives notifications as a job description and returns 202 Accepted, then processes said notification to a fanout row

The fanout row contains the actual state of a fanout (that is, the cursor of which user the notification has last been sent). an api instance locks a fanout and reliably creates the messages (as possible). When the fanout is finished, its row is deleted and the job is set as dispatched

The workers are listening on the corresponding queues. They receive the messages and process them against the provider accordingly (with retries, deadletter, circuit breaker), thus emptying the quorum queues

The workers provide metrics for the ongoing jobs and scale down as needed when the queues are quiet or empty

### Useful documentation, tips
* Check `docs/diagrams` for useful diagrams on the system
* Check `docs/prompts` to see most of the prompts that were used in this challenge
* The `plans` and other .md files are rough sketches, not really indicative of the system, but kept for completeness
* The frontend is a useful testing tool. It exposes documentation and examples through `redoc`, has functionality and an ad-hoc dashboard (improvised frontend for prom metrics)
* Check the 'compose' and 'k8s' deploy scripts at the README or browse if curious!
* There is a grafana for APM and log visibility, even though it needs some love

### On AI use
Everything was made on claude code CLI using opus 5.5 (effort high) in my personal license. Claude tried to slop hard every time and I had to prioritize simplicity, removing fluff and entire parts of the system which weren't really needed or useful

I had claude summarize the APOSD book and use it as a skill to check for antipatterns

I gathered google's best practices on go and have claude create a skill for more idiomatic code

### On slop
The domain code has been mostly read and understood, simplifying abstractions for the sake of the exercise while maintaining important parts 'mature'. I tried to ensure scalability and correct technical tradeoffs as the main subject of this demo

The go project structure is not pristine but reflects a basic working set. The projects use layers, have tests, dependencies, config (envs), and a small k8s 'production' emulation to demonstrate live scaling and configuration

The infra, observability stack and its related code contains some slop and is not so mature. Even though the ideas and premise were provided by me, in a real world setting I would probably abstract it better, be more careful and use an observability commons/library

The frontend/monitoring part is accomplished and works, but a production scenario would require hardening and more complex parts

### Some potential improvements
* Versioning of user lists (what happens if CUD on list while a fanout is in progress?)
* Centralized, simple circuit breaker using atomic redis counters and fixed buckets
* More robust observability code, composable
* Hashing of app ID for queues, not 1 queue per app ID (if 1000 tenants, 1000 queues?)
* Partitioning and persistance of metrics. More advanced business insights for admins
* What about in-app notifications, and pull notifications (an entirely different problem, but valid question)?
