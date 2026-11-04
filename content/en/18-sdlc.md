# SDLC and methodologies: Waterfall, Agile, Scrum, Kanban

How the software development life cycle works and how teams organise their work: the SDLC phases, models from waterfall to Agile, Scrum and Kanban in detail with their roles, events and metrics, where DevOps and SRE fit into the process, working with requirements and estimates, change management, testing and documentation. These questions come up in almost every interview, especially for middle and senior roles and at large companies with formal processes.

## What SDLC is and why it is needed

**SDLC (Software Development Life Cycle)** is the sequence of stages a software product goes through: from an idea to running in production, then ongoing support or decommissioning.

Why it is formalised:
- **predictability** — it is clear what is done, what remains and when there will be a result;
- **quality** — mistakes are caught at early stages, where fixing them is cheaper (an error in requirements found in production costs dozens of times more than one found during discussion);
- **division of responsibility** — who writes requirements, who designs, who tests, who deploys;
- **risk management and compliance** (audits, regulators, security).

An SDLC model does not answer "which stages exist" (they are almost always the same) but **"in what order and in what portions to go through them"**: once, end to end (waterfall), or in many short cycles (Agile).

## SDLC phases

### 1. Planning and requirements analysis
What happens: understand **which problem we are solving and for whom**. Gather requirements from the business and users, assess feasibility, timelines, cost and risks.
- **Functional requirements** — what the system must do: "a user can place an order", "the report can be exported to Excel".
- **Non-functional requirements** — what the system must be like: 99.9% availability, responses under 300 ms, handling 1000 requests per second, storing data in the EU, PCI DSS compliance. These are the ones that concern DevOps and SRE the most.

Artefacts: business requirements, a specification (SRS — Software Requirements Specification, a document fully describing the system's requirements), user stories (requirements in the form "as a <user> I want <action> so that <benefit>"), time and budget estimates.

The DevOps/SRE role: at this stage it is important to set non-functional requirements up front — SLOs, resilience, security and infrastructure cost requirements. If they are not discussed now, they will be "remembered" a week before the release.

### 2. Design
What happens: decide **how** the system will be built.
- **HLD (High-Level Design)** — the architecture in large blocks: services, databases, queues, interactions, the cloud, the network layout.
- **LLD (Low-Level Design)** — the details: database schemas, API contracts, classes and modules.

Artefacts: architecture diagrams, ADRs (architecture decision records), API specifications (OpenAPI — the standard YAML or JSON format for describing HTTP APIs, from which documentation and clients are generated), a threat model (an analysis of who could attack the system, how, and what protects against it).

The DevOps role: choosing the infrastructure, the environment layout, the deployment approach, observability, resilience, cost estimates.

### 3. Implementation
Code is written according to the design. Practices: code review via merge requests, coding standards, linters, unit tests, branching (trunk-based or GitFlow), continuous integration.

The DevOps role: CI pipelines, service templates, local and temporary environments for developers, infrastructure as code.

### 4. Testing
Verify that the system does what is needed and does it reliably: functional tests, integration tests, end-to-end tests, load tests, security tests, user acceptance testing (UAT).

The DevOps role: automating tests in the pipeline, test environments, test data, load-testing setups.

### 5. Deployment (release)
Shipping to production: building the artefact, rolling out through environments, the deployment strategy (rolling, blue/green, canary), database migrations, a rollback plan, communication with users.

The DevOps role: this is the core area of responsibility — CD pipelines, GitOps, automatic rollback, release management.

### 6. Operations and maintenance
The system runs in production: monitoring, alerts, incidents, bug fixes, dependency and security updates, scaling, cost optimisation. Eventually — decommissioning and data migration.

The DevOps and SRE role: observability, on-call, SLOs, postmortems, automating routine work. Feedback from here (incidents, metrics, user complaints) goes back into planning — the cycle closes.

### Why it matters that this is a cycle
The most expensive mistake is to treat SDLC as a straight line: "delivered the project and forgot about it". A system spends most of its life in operation, and that is exactly where the knowledge of what to improve comes from. Agile and DevOps are essentially about **going through this cycle as often and as briefly as possible**.

## SDLC models

### Waterfall
The phases go **strictly in sequence**, each starting when the previous one is finished: requirements → design → implementation → testing → deployment → maintenance. Going back to a previous stage is an exception and is expensive. Each phase ends with a formal document and sign-off.

Pros:
- a clear plan, timeline and budget from the start;
- detailed documentation;
- easy to control stage by stage, suits fixed-price contracts.

Cons:
- **a working product appears only at the very end** — if the requirements were misunderstood, it will turn out months later;
- changing requirements along the way is very expensive;
- testing at the end: defects are found late and pile up;
- the risk of "integration hell", when parts developed separately are put together for the first time.

When it fits: requirements are stable and well known in advance, the cost of a mistake is very high, formal control is required — embedded software, medical and aviation systems, government contracts, physical infrastructure (building a data centre, where you cannot work "in iterations").

### The V-model
An evolution of waterfall: **each development phase has a matching level of testing**, and the tests are planned at the same time as the corresponding phase.
| Development phase | Matching testing |
|---|---|
| Requirements | Acceptance testing (UAT — User Acceptance Testing: the customer or users themselves verify that the system solves their problem) |
| System architecture | System testing |
| Module design and their interaction | Integration testing |
| Code | Unit testing |

The idea: acceptance test cases are written while the requirements are being defined, so unclear requirements are spotted earlier. It is used in the same places as waterfall, especially where verification requirements are strict.

### Iterative and incremental models
- **Incremental** — the product is split into parts (increments); each goes through the full cycle and is added to the already working product: first sign-up and the catalogue, then the cart, then payment.
- **Iterative** — the product is improved in repeated cycles: first a simplified version of everything, then each iteration refines it.

In practice they are combined, and this combination is the foundation of Agile.

### The spiral model
Each turn of the spiral is a cycle of "planning → **risk analysis** → development and testing → customer evaluation". The key idea is to eliminate the biggest risks first on each turn (prototype the unclear parts). It suits large, expensive and risky projects. It is rare today, but its idea of risk management lives on in all modern approaches.

### Agile
**Agile** is not a specific process but **a set of values and principles** formulated in the Agile Manifesto (2001). Specific implementations are Scrum, Kanban, XP and others.

The four values of the manifesto (the items on the right matter too, but the ones on the left matter more):
1. **Individuals and interactions** over processes and tools.
2. **Working software** over comprehensive documentation.
3. **Customer collaboration** over contract negotiation.
4. **Responding to change** over following a plan.

Key principles (out of twelve): deliver working software frequently — every few weeks, preferably more often; welcome changing requirements, even late in development; business people and developers work together daily; working software is the primary measure of progress; continuous attention to technical excellence; simplicity — the art of not doing unnecessary work; self-organising teams; regular reflection and adjusting the process.

The essence in one sentence: **short cycles, frequent feedback, adapting the plan as you go** instead of trying to foresee everything up front.

A common misunderstanding: Agile does not mean "no documentation and no planning". There is a plan, but it is short and regularly revisited; there is documentation, but as much as is actually needed.

## Scrum

Scrum is the most widespread Agile framework. Work happens in **sprints** — fixed-length iterations (usually 2 weeks, from 1 to 4), and each ends with a potentially releasable increment of the product.

### Roles
- **Product Owner** — responsible for the product's value: maintains and prioritises the backlog, decides **what** to do and in what order. One person, not a committee.
- **Scrum Master** — responsible for the process working: helps the team, removes impediments (blockers), runs the events, protects the team from constant outside interruptions. Not a manager or the team's boss.
- **Developers** — everyone who does the work: developers, testers, DevOps engineers, designers. They decide themselves **how** to do the work and how much to take into the sprint. Usually 3–9 people.

### Events
| Event | When and how long | Why |
|---|---|---|
| **Sprint Planning** | At the start of the sprint, up to 4 hours for 2 weeks | Choose items from the backlog and set the sprint goal (Sprint Goal) |
| **Daily Scrum** | Every day, 15 minutes | Synchronise: progress towards the sprint goal, impediments. Not a report to the boss but the team planning its day |
| **Sprint Review** | At the end of the sprint | Show what was done to stakeholders (a demo), gather feedback, adjust the backlog |
| **Sprint Retrospective** | After the Review | Discuss the process: what went well, what got in the way, what to change next sprint |
| **Backlog Refinement** | During the sprint | Clarify and refine upcoming items, estimate them, split large ones |

### Artefacts
- **Product Backlog** — an ordered list of everything the product might need.
- **Sprint Backlog** — the items chosen for the sprint and the plan for doing them.
- **Increment** — the result of the sprint: a working, verified product ready for release.

### Definition of Ready and Definition of Done
- **Definition of Ready (DoR)** — when an item can be taken into work: the goal is clear, there are acceptance criteria, it is estimated, there are no blocking dependencies.
- **Definition of Done (DoD)** — when an item is considered **done**. An example for a service: the code has passed review; tests are written and pass in CI; deployed to stage; metrics and alerts added; documentation and runbook updated; the security check passed. Without a shared DoD everyone has their own "done", and "almost finished" items pile up.

A DevOps engineer can stand out in an interview by saying that the team's DoD includes **observability and operational readiness**, not just "the code is written".

### Scrum estimation and metrics
- **Story points** — a relative estimate of an item's complexity, size and uncertainty, not hours. Usually on a Fibonacci scale (1, 2, 3, 5, 8, 13): the larger the item, the rougher the estimate. An item of 13 or more is better split.
- **Planning poker** — everyone shows their estimate at the same time; large differences are discussed — this surfaces hidden complexity and different understandings of the item.
- **Velocity** — how many story points the team completes per sprint on average. It is used **for the team's own planning**, not to compare teams and not as a KPI (otherwise estimates start to "inflate").
- **Burndown chart** — a chart of the remaining work in the sprint by day: shows whether the team is on track.

### How DevOps and platform work fit into Scrum
The main difficulty: operational work is **unpredictable** — incidents, urgent requests, on-call. It cannot be planned two weeks ahead. How teams cope:
- a **capacity buffer** in the sprint — for example, 20–30% for unplanned work;
- a rotating **interrupt handler**: one person per sprint takes all incoming requests and incidents while the rest work calmly towards the sprint goal;
- the operational part of the team moves to **Kanban** while the project part stays on Scrum, or the whole team works with **Scrumban**.

## Kanban

Kanban comes from the Toyota Production System. In software it is a method for managing **a continuous flow of work** without fixed iterations.

### Core practices
1. **Visualise the work** — a board with columns reflecting the process stages, for example: Backlog → Ready → In Progress → Review → Testing → Done. Each item is a card; you can see where everything is and where it is stuck.
2. **Limit work in progress (WIP limits)** — a column has a maximum number of cards, for example "In Progress: no more than 3". If the column is full, no new item is taken; instead people help finish the ones already started.
3. **Manage the flow** — watch how quickly items move through the system and look for bottlenecks.
4. **Make process policies explicit** — what "ready for review" means, who moves a card and when.
5. **A pull system** — work is not "pushed down" from above; a person **pulls** the next item when they become free.
6. **Continuously improve** the process based on data.

### Why WIP limits work
It feels intuitive that the more items you take on at once, the more you will finish. In reality it is the opposite:
- switching between items eats time and attention;
- items started at the same time finish later than if they were done one after another;
- unfinished work delivers no value, and hanging branches go stale and conflict.

A WIP limit forces you to **finish before you start** ("stop starting, start finishing") and immediately exposes the bottleneck: if the Review column fills up, the problem is review, not development.

### Flow metrics
- **Lead time** — the time from a request appearing (or being accepted) to its release. What the customer sees: "I asked — how long until I got it".
- **Cycle time** — the time from **starting** work on an item to finishing it. What the team itself controls.
- **Throughput** — how many items are finished per period (per week).
- **CFD (Cumulative Flow Diagram)** — a cumulative chart of the number of items in each status over time. A widening band shows where work is piling up (a bottleneck).

**Little's law** links them: **average time in progress = work in progress / throughput**. If a team finishes 5 items a week on average and has 20 in progress at once, each item will take about 4 weeks. If you want things faster, reduce WIP rather than "work faster". This is a strong interview argument for WIP limits.

### Classes of service
Different items need different speeds:
- **Expedite** — an incident in production: jumps the queue and may exceed the WIP limit, but there is at most one such item at a time;
- **Fixed date** — with a hard deadline (a certificate expiring, a mandatory update);
- **Standard** — regular work in priority order;
- **Intangible** — tech debt and improvements that are not urgent, but if they are never done it will hurt later.

### Why Kanban suits ops and platform teams
The flow of work is unpredictable (incidents, requests from other teams), items vary a lot in size, and waiting for the next sprint to start an urgent item makes no sense. Kanban lets you take work as it arrives while still controlling the load through WIP.

## Other approaches

### Scrumban
A hybrid: regular Scrum meetings (planning, retrospectives) and a Kanban board with WIP limits and continuous flow instead of a hard sprint commitment. A common choice for teams that mix project and operational work.

### Extreme Programming (XP)
An Agile methodology focused on **engineering practices**: pair programming, test-driven development (TDD), continuous integration, small frequent releases, refactoring, collective code ownership, simple design. Much of XP (CI, TDD, refactoring) has become standard even without formal "XP".

### Lean
Lean manufacturing principles applied to software: **eliminate waste** (partially done work, extra features, waiting, task switching, hand-offs between teams, defects), speed up the flow, defer irreversible decisions to the last responsible moment, build quality in, respect people. Kanban is one of the Lean tools.

### Scaling Agile: SAFe, LeSS
When dozens of teams work on one product, coordination is needed:
- **SAFe (Scaled Agile Framework)** — a heavily prescribed scheme: teams are grouped into "trains" (an Agile Release Train — 5–12 teams working on one product and releasing it together), and once a quarter there is a joint planning session (PI Planning — a two-day meeting of all the train's teams to agree goals and dependencies for the quarter). Popular in large corporations, criticised for bureaucracy.
- **LeSS (Large-Scale Scrum)** — minimalist scaling of Scrum: one product, one Product Owner, one backlog for several teams.

## DevOps and SRE as a continuation of Agile

Agile sped up development but often stopped at the "code is written" boundary: after that a release waited for weeks because operations was a separate department with its own goals (developers want change, operations wants stability). **DevOps** extends the ideas of Agile to the whole path to production and to operations.

**DevOps** is a culture and a set of practices in which development and operations work together and share responsibility for the product in production. The **CALMS** model describes its components:
- **Culture** — shared responsibility, no blame, collaboration;
- **Automation** — CI/CD, infrastructure as code, automated tests;
- **Lean** — small batches of change, eliminating waiting and hand-offs;
- **Measurement** — metrics (DORA, SLOs) and decisions based on them;
- **Sharing** — sharing knowledge and tools between teams.

**The Three Ways of DevOps** (from "The Phoenix Project"): flow (speed up work moving from idea to production), feedback (learn about problems quickly), continuous learning and experimentation.

**DORA metrics** are the measurable outcome: deployment frequency, lead time from commit to production, change failure rate, time to restore. Research shows the best teams are both faster **and** more stable — speed and reliability do not contradict each other when changes are small and automated.

**Shift left** — move checks as early in the cycle as possible: tests, security (DevSecOps — SAST, dependency and image scanning in CI), infrastructure checks (Terraform linters, policy as code), load testing. A defect caught in a merge request costs minutes; in production — hours and reputation.

**SRE** is Google's concrete implementation of DevOps: operations as an engineering problem, reliability measured by SLOs, and the speed-versus-stability balance managed through the error budget (covered in detail in the SRE topic). People often say "class SRE implements DevOps".

**Platform engineering** is the next step: a dedicated platform team builds an internal platform (service templates, CI/CD, on-demand environments, observability out of the box) as a product for developers, so that every team does not have to figure out Kubernetes and Terraform on its own. A "golden path" is the recommended and supported way to do a typical thing.

## Requirements, work items and estimates

### The work hierarchy
- **Epic** — a large goal spanning several sprints: "move the services from the data centre to AWS".
- **User story** — a small piece of value for a user that fits into a sprint.
- **Task** — a technical part of a story: "write a Terraform module for RDS".
- **Bug** and **spike** — a time-boxed investigation to remove uncertainty ("find out within 2 days whether Karpenter suits our workloads").

### How to write a user story
The template: **"As a <role>, I want <action>, so that <value>"**. For example: "As a developer, I want to create a temporary environment for my merge request, so that I can test changes without disturbing others".

**Acceptance criteria** are verifiable conditions of done, often in the form "Given — When — Then": "Given: an MR is open. When: the pipeline has passed. Then: the MR comment has a link to the environment, and the environment is deleted after the MR is closed".

**INVEST** — the qualities of a good story: Independent, Negotiable, Valuable, Estimable, Small, Testable.

### Prioritisation
- **MoSCoW** — Must have, Should have, Could have, Won't have this time.
- **A value-versus-effort matrix** — first the items with high value and low effort ("quick wins").
- **RICE** — Reach × Impact × Confidence / Effort.
- **WSJF** (in SAFe) — the cost of delay divided by the job's duration.

## Change management and ITIL

In large companies (banks, telecoms, the public sector) changes to production go through a formal process. This is often asked, especially if the company works according to **ITIL** — a set of IT service management practices.

The main ITIL processes you need to tell apart:
- **Incident management** — restore the service as quickly as possible. The goal is to bring the service back, not to find the cause.
- **Problem management** — find and eliminate the **root cause** of recurring or major incidents so they do not happen again.
- **Change management** (change enablement in ITIL 4) — plan and control changes so they do not break services.
- **Service request** — standard user requests (grant access, create an environment).

**Types of change:**
- **Standard** — pre-approved, low-risk, repeatable (a typical deploy through a proven pipeline, adding a user). They need no separate approval.
- **Normal** — require a risk assessment and approval, often at a **CAB (Change Advisory Board)** — a meeting where team representatives review upcoming changes.
- **Emergency** — urgent (fixing an ongoing incident), approved in an expedited way and documented afterwards.

**How to combine this with DevOps.** A weekly CAB for every deploy kills speed. The modern approach is to make **an automated pipeline with checks a standard change** — tests, scanning, review in the MR, gradual rollout and automatic rollback give more assurance than a meeting. Manual approval remains for the truly risky: database migrations, network and permission changes. DORA research shows that external change approval does not reduce the change failure rate but slows delivery down.

**Change windows and change freezes** are periods when production changes are forbidden or restricted: holidays, Black Friday, quarter-end for finance.

### Releases and versions
- **Semantic versioning (SemVer)**: `MAJOR.MINOR.PATCH`. MAJOR — incompatible API changes, MINOR — backwards-compatible new features, PATCH — fixes.
- **Release notes and a changelog** — what went into a version; convenient to generate automatically from Conventional Commits.
- **A deploy and a release are not the same thing.** A deploy means the code is rolled out to production; a release means the functionality has become available to users. With **feature flags** they can be separated: roll the code out disabled and switch it on later, for some users.
- **Environments:** dev → test (QA) → stage (as close to production as possible) → prod. The same artefact is promoted through the environments.

## Testing in the SDLC

**The test pyramid:**
- **the base — unit tests**: many, fast (milliseconds), checking individual functions;
- **the middle — integration tests**: fewer, checking how components interact (a service and a real database, a service and a queue), contract tests for APIs between services;
- **the top — end-to-end (E2E)**: few, slow and brittle, checking whole user scenarios through the interface.

The "ice cream cone" anti-pattern — almost everything is checked manually or by E2E tests: the pipeline takes an hour and constantly fails on flaky tests.

**Kinds of testing that concern DevOps and SRE:**
- **load** testing — can the system handle the expected load; a **stress test** — where the limit is and how the system degrades; a **soak test** — sustained load over a long time (memory leaks);
- **security testing** — SAST (code analysis), DAST (attacks on the running application), dependency and image scanning, penetration testing;
- **resilience testing** — chaos engineering, game days, verifying restores from backup (a backup that has never been restored from does not count as a backup);
- **smoke tests** after a deploy — a basic check that the service is up and the main scenarios work;
- **infrastructure testing** — `terraform plan` and policy checks in the MR, Molecule for Ansible, `terraform test`.

## Documentation
Agile does not abolish documentation; it requires it to be **useful and up to date**:
- **ADR (Architecture Decision Record)** — a short record of an architectural decision: the context, the options considered, the decision and its consequences. A year later it explains **why** Aurora was chosen rather than DynamoDB.
- **RFC / design doc** — a proposal for a large change, discussed before the work starts.
- **Runbook** — an operational guide: how to diagnose and fix a specific problem; every alert links to one.
- **README and API documentation** (OpenAPI) — next to the code, updated in the same MR as the code (docs as code).
- **Postmortems** — what happened and what was changed.

## How to talk about this in an interview
The interviewer does not care about the definition of Scrum but about **how you worked** and whether you understand why a process exists. Prepare answers about:
- which methodology the team used, the sprint length, which meetings there were;
- how planned work and operations were combined (a buffer, an interrupt handler, Kanban for ops);
- what was in the Definition of Done, whether you took part in estimating;
- how releases and change approval were organised;
- what you proposed to improve in the process at retrospectives and what came of it.

## Interview questions

**What is the SDLC and which phases does it consist of?**
The software development life cycle: planning and requirements analysis, design, implementation, testing, deployment, operations and maintenance. It is a cycle: feedback from operations goes back into planning. SDLC models differ in the order and the portions in which these phases are gone through.

**How does Waterfall differ from Agile?**
Waterfall goes through the phases in sequence and once: the plan and budget are clear up front, but a working product appears only at the end and changes are expensive. Agile goes through the cycle many times in short iterations: the product ships often, feedback comes early, and the plan adapts. Waterfall fits stable requirements and a high cost of error; Agile fits uncertainty and the need to react quickly.

**When is waterfall the right choice?**
When requirements are stable and known in advance, changes along the way are impossible or very expensive, and formal control and detailed documentation are required: embedded and medical systems, aviation, fixed-scope government contracts, physical infrastructure.

**What roles, events and artefacts does Scrum have?**
Roles: Product Owner (what to do and in what order), Scrum Master (making the process work), Developers (how to do it). Events: the Sprint, Sprint Planning, Daily Scrum, Sprint Review, Sprint Retrospective, plus Backlog Refinement. Artefacts: Product Backlog, Sprint Backlog, Increment.

**How does the Sprint Review differ from the Retrospective?**
The Review is about the product: show the result to stakeholders and gather feedback. The Retrospective is about the process: what to improve in the team's way of working next sprint.

**What is the Definition of Done and why is it needed?**
A team-wide list of conditions under which an item counts as done: review, tests in CI, deploy to stage, metrics and alerts, documentation. Without it everyone has their own "done", and almost-finished items pile up. For DevOps it matters that operational readiness is part of the DoD.

**What are story points and velocity? Can teams be compared by velocity?**
Story points are a relative estimate of an item's complexity and uncertainty, not hours. Velocity is how many points the team completes per sprint and is used for its own planning. Teams cannot be compared: each has its own scale, and turning velocity into a KPI leads to inflated estimates.

**How does Kanban differ from Scrum?**
Scrum has fixed sprints with a sprint commitment, roles and mandatory events. Kanban is a continuous flow without iterations, limiting work in progress with WIP limits, work is pulled as people free up, and there are no mandatory roles. Scrum suits product development with plannable scope; Kanban suits a flow of unpredictable work (support, operations, platform teams).

**Why are WIP limits needed?**
They force finishing what was started rather than starting something new: fewer context switches, items move faster, and the bottleneck is immediately visible (where cards pile up). By Little's law, the average time in progress equals work in progress divided by throughput, so reducing WIP directly shortens delivery time.

**How does lead time differ from cycle time?**
Lead time runs from a request appearing to its release — what the customer sees. Cycle time runs from starting work on an item to finishing it — what the team itself controls. The difference between them is the time spent waiting in the queue.

**How do you combine sprint work with incidents and urgent requests?**
Reserve capacity for unplanned work (20–30%), assign an interrupt handler who takes all incoming requests, or run operational work on Kanban (Scrumban). Then analyse where the stream of interruptions comes from and remove its causes through automation.

**How is DevOps related to Agile?**
Agile sped up development, and DevOps extended the same ideas — small batches, frequent feedback, shared responsibility — to the path to production and to operations: CI/CD, infrastructure as code, observability, development and operations working together. The CALMS model: culture, automation, Lean, measurement, sharing.

**What is shift left?**
Moving checks to the early stages of the cycle: tests, security, infrastructure and policy checks run in the merge request pipeline rather than before release or in production. A defect found earlier is cheaper and faster to fix.

**How does a deploy differ from a release?**
A deploy is rolling code out to production. A release is making functionality available to users. Feature flags separate them: the code is rolled out disabled and switched on later, gradually or for some users, and a rollback means turning the flag off rather than redeploying.

**How does incident management differ from problem management?**
Incident management restores the service as quickly as possible, even with a workaround. Problem management finds and eliminates the root cause so the incident does not recur.

**What is a CAB, and how do you combine change approval with continuous delivery?**
A Change Advisory Board is a meeting where risky changes are approved. To avoid slowing delivery, an automated pipeline with tests, review, gradual rollout and automatic rollback is registered as a standard, pre-approved change, and manual approval is kept for the truly risky operations (database migrations, network and access changes).

**What is the test pyramid?**
Many fast unit tests at the base, fewer integration tests in the middle, a few slow end-to-end tests at the top. This keeps the pipeline fast and stable. An inverted pyramid (everything checked by E2E or manually) makes the pipeline slow and unreliable.

**What is an ADR and why is it needed?**
An Architecture Decision Record is a short record of an architectural decision: the context, the options, the decision and its consequences. It lets newcomers, and yourself a year later, understand why the system is built the way it is and avoid revisiting decisions in circles.

**Which methodology did your team use, and what would you change about it?**
Prepare a concrete answer: the methodology, the sprint length, the meetings, how planned and operational work were combined, what was in the Definition of Done, how releases worked. And one or two real improvements you proposed at retrospectives, with the outcome.
