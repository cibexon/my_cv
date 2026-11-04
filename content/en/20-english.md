# English: the HR interview

Preparing for the English check: how it goes, useful phrases and my answers to typical questions. The answers I wrote myself are translated into natural spoken English; the others are drafts to adjust (places for my own details are marked `[in square brackets]`). In the "❓ All questions" tab the questions of this topic work as practice cards: read the question, answer out loud, then open the answer.

## How the English check goes

- Usually it is a **short conversation of 5–15 minutes** with HR or a language coach — at the start of screening, as a separate call or right in the middle of a technical interview ("let's switch to English").
- They assess not your knowledge of terms but **whether you can talk freely about your work**:
  - **fluency** — speaking without long pauses, not stopping at every word;
  - **comprehension** — understanding questions the first time;
  - **vocabulary** — having enough words to explain your thought;
  - **grammar** — especially past tenses when talking about experience (*I worked, I built, we migrated*);
  - **pronunciation** — being understandable, not a perfect accent.
- The result is usually recorded as a level on the CEFR scale (the Common European Framework of language proficiency, from A1 to C2): B1 (intermediate — you get by in familiar situations), B2 (upper-intermediate — you discuss work topics freely, though with mistakes), C1 (advanced — you speak flexibly and precisely on almost any topic). Most DevOps roles with international teams need **B2**.
- The questions are almost always the same, so you can prepare well in advance.

## How to answer: structure and useful phrases

**Answer structure.** A short answer to the question → an example from experience → the outcome. For stories, use STAR: *Situation* → *Task* → *Action* (what **I** did) → *Result* (preferably with numbers). One answer — 1–2 minutes.

**Don't memorise the text word for word.** A memorised text is obvious immediately, and if you lose your place it is hard to continue. Remember **the structure and key phrases** of each answer and tell it in your own words.

**If you didn't understand the question:**
- *Sorry, could you repeat that, please?*
- *Could you rephrase the question?*
- *Do you mean…?* / *Just to make sure I understand — you're asking about…?*

**If you need to think:**
- *That's a good question. Let me think for a second.*
- *Let me give you an example.*

**Linking parts of an answer:**
- *First… Then… After that… Finally… As a result…*
- *For example… / For instance…*
- *To be honest… / Actually…*
- *On the one hand… On the other hand…*

**If you forget a word** — describe it in other words (*"the thing that…"*, *"a tool that…"*). That is fine and shows fluency.

**How to practise:** record your answers on your phone and listen to them; speak out loud for 10–15 minutes a day; watch your past tenses when talking about experience.

## What to fix in the answers before the interview

- **Dates must match the CV.** The answers say "the last 5 years at Temabit", but the CV lists Temabit as November 2022 – November 2026, i.e. 4 years (and EPAM as 2021–2022). HR compares the story with the CV, so it is better to say *"the last four years"* or the general *"more than five years in DevOps"*. The story also mentions A2 Hosting, which is not in the CV — either add it to the CV or leave it out.
- **Better not to say "sometimes I'm lazy"**, even as a joke: the interviewer may miss the irony, especially in a foreign language. The same idea can be presented as a strength — *"I don't like doing the same manual work twice, so I automate it"* (there is a version in the answer below).
- **The question about layoffs because of AI** may sound anxious. It is safer to ask how the company uses AI and how it affects the team's work (version below).
- **Age and family in the introduction** — acceptable but not necessary; one short sentence is enough, then move straight to experience.
- **End the reason for looking for a job on a positive note:** after mentioning the layoffs, add what you want next.
- **Add a result with numbers** to the migration story: how many services, how much downtime, what improved (faster deployments, cheaper infrastructure).

## Interview questions

**Can you tell me about yourself?**
My name is Alex. I'm 40, happily married and a father. Since we're talking about my professional background, let me tell you a bit about my experience in IT.

After graduating, I started working in retail — not in my field at all. Then friends told me about a hosting company called JustHost, and that's where my IT career began. The company had several departments, and our team did technical support: domain registration, DNS, customer chats and all kinds of low-level technical tasks.

That project closed fairly soon, and I moved to Hostpro, a Ukrainian hosting provider. I spent about five years there and grew from L1 support to technical team lead. That's where I really got comfortable with Linux and Unix systems and Bash scripting. As a team lead, I was responsible for the quality of the L1 team's work with customers, and when needed I got involved in investigating complex issues and deciding on compensation for customers.

After that, I joined an interesting Israeli company called Vimmi, which built video and audio streaming for TV and radio. I was a technical administrator there: I monitored the streams, recovered lost or corrupted data, worked with MySQL replication and so on.

Then I went back to hosting, but this time for international companies — HostPapa and A2 Hosting. The market was full of WordPress sites, so I did a lot of WordPress migrations, and I also investigated and cleaned up hacked sites and fixed all kinds of technical problems with them. At the same time, I started learning cloud and Docker technologies, and that's how I got to EPAM, where I worked on a couple of projects.

My most recent job was at Fozzy Group, a large holding company, in Temabit's DevOps team of about 50 engineers. I spent the last [four] years there as a DevOps/SRE engineer, working with many different technologies, and one of our main goals was moving the infrastructure from on-premise to AWS. I'm happy to go into detail on any of the projects, but in short:

On-premise, I built and supported Docker Swarm clusters with Ansible. We chose a clustered setup for fault tolerance: the stack included HAProxy, Pacemaker, GlusterFS and a virtual IP, with Docker Secrets and Vault for secrets. I also built CI/CD in TeamCity covering the full cycle — from the build to running the application on the cluster, with Nexus as the registry. The applications were mostly .NET Core, PHP with nginx, and Node.js. Later we moved from Docker Swarm to on-premise Kubernetes clusters.

For the migration to AWS, I set up the infrastructure with Terraform — EKS clusters with GitOps through ArgoCD — and built GitLab CI/CD pipelines covering the full cycle up to deployment on the cluster, with ECR as the registry. I also worked on separate projects where I planned the architecture, deployed it and then supported it, both on the technical side and on FinOps — keeping cloud costs under control.

**What is your current or most recent project, and what was your role?**
My most recent job was at Fozzy Group, in the Temabit team, where I worked as a DevOps/SRE engineer for the last [four] years, with many different technologies and a migration from on-premise infrastructure to AWS.

Over these years there were many projects and a lot of platform work. My last project was Fozzyshop, an online store. It had a public front end — the website itself, running on PHP-FPM, nginx and MySQL — and a back end with a large private microservice architecture.

My job was to design, implement and support the project's infrastructure and to improve it overall. After the war started, I was also responsible for moving it to AWS step by step.

**What does a typical day look like for you?**
Draft — adjust it to yourself.

My day usually starts with checking monitoring and alerts to make sure everything is fine in production. Then I work on planned tasks — for example, Terraform changes, improving CI/CD pipelines or preparing a migration. During the day I also help developers with deployments and troubleshooting. Our team had [N] DevOps engineers, and we worked closely with [N] development teams. I was also part of the on-call rotation.

**Can you tell me about a challenging task you solved?**
One of the biggest tasks I had to solve was migrating the Fozzyshop project from our on-premise infrastructure to AWS. It was a complex task with several stages.

First, I designed the project's architecture in AWS. Then I configured the test environment, and we tested everything there. After that, we built the production environment: we configured it, synchronised the data and switched the traffic to the AWS infrastructure.

As a result, [the shop moved to AWS with minimal downtime / we migrated N services / deployments became X times faster / costs went down by X%].

Tip: be sure to end with a concrete result — it is the most important part of a STAR answer.

**Why are you looking for a new job?**
Unfortunately, Fozzy Group recently suffered huge losses because of the war in Ukraine, and as a result there were significant staff cuts. So right now I'm looking for a new job.

Tip: add a positive ending, for example: *"Now I'm looking for a role where I can work with modern cloud infrastructure and keep growing in platform engineering and AI infrastructure."*

**Why are you interested in this position?**
Draft — adjust it to the specific company.

I'm interested in this position because it matches my experience well — [AWS, Kubernetes, CI/CD, migrations] — and it also gives me a chance to grow in [platform engineering / large-scale infrastructure / AI infrastructure]. I read that your company [one fact about the company or product], and I like that [the product / the scale / the team's approach]. I think my experience with [migration to AWS / high-load EKS clusters / observability] can be useful for your team.

**What are your strengths and weaknesses?**
I'm sociable and a team player. I can adapt to different situations and find common ground with people. On the technical side, I have a lot of experience with Linux and troubleshooting.

As for weaknesses, I'd say programming languages like Python or Go — I mostly use Bash. And sometimes I can be a bit lazy — but that can also be a strength, because, as you know, laziness is the engine of progress.

Recommended version without "lazy": *"As for weaknesses, my main language for automation is Bash, and I'm improving my Python now. Also, I really don't like doing the same manual work twice — so I usually automate it, which sometimes means I spend extra time on automation at the start, but it saves a lot of time later."*

**How do you work with your team and with developers?**
Draft — adjust it to yourself.

I try to work as a partner for developers, not as a gatekeeper. For example, I build CI/CD templates and documentation so that teams can deploy on their own, and I help them when something breaks in production. We discuss bigger changes in merge requests and short meetings, and I always try to explain why something is needed — for example, a security rule or a resource limit — instead of just saying "no".

**Can you tell me about a disagreement with a colleague and how you handled it?**
Draft — replace it with your real story.

Once a developer wanted to [deploy a change directly to production / skip a review] because of a deadline, and I didn't agree because it was risky. Instead of arguing, I explained the risk and suggested a compromise: [we deployed to the test environment first and then to production with a quick rollback plan]. In the end, we delivered on time, and the team agreed to use this approach for urgent releases.

**Can you explain what Kubernetes is to a non-technical person?**
Imagine you have hundreds of applications and many servers. Kubernetes is like an automatic manager for them. You just tell it, "I need three copies of this application running," and it decides on which servers to run them. If one copy crashes, Kubernetes restarts it, and if a whole server fails, it moves the applications to other servers. So people don't have to do all of this manually, day and night.

**Can you explain what CI/CD is in simple words?**
CI/CD is like an automatic assembly line for software. When a developer changes the code, the line automatically builds the application, runs tests to check that nothing is broken and then delivers the new version to the servers. Without it, people do these steps by hand, which is slow and easy to get wrong. With CI/CD, we can release small changes many times a day, quickly and safely.

**Can you explain what Terraform is in simple words?**
Terraform lets us describe our infrastructure — servers, networks, databases — in text files, like a blueprint or a recipe. Then Terraform creates everything exactly as described. If we need the same environment again, we just run the same recipe. And because the files are stored in Git, we can see who changed what and when, and review changes before they go live.

**What do you do in your free time?**
Draft — adjust it to yourself.

Most of my free time I spend with my family — I have a little son, so there's never a dull moment. I also like [hobby: sport / walking / cooking / games]. And honestly, I enjoy learning new technologies — right now I'm playing with AI tools and building small projects at home.

**How was your weekend?**
Draft — adjust it to yourself.

It was nice, thank you. I spent it with my family — we [went for a walk / visited friends / stayed at home]. I also found some time to [prepare for my AWS exam / read about Kubernetes]. How about you?

**Where are you from, and where do you live now?**
Draft — adjust it to yourself.

I'm from Ukraine. I live in [city] with my family. I work remotely, so I have a comfortable home office.

**What are you learning now?**
Right now I'm preparing for the AWS Solutions Architect Associate exam and for the CKA — Certified Kubernetes Administrator.

You can add: *"I'm also learning how to run AI systems in production — things like RAG and the Model Context Protocol. I've already integrated an AI agent with Jira and Confluence using MCP."*

**Where do you see yourself in three to five years?**
Draft — adjust it to yourself.

In three to five years I see myself as a senior platform or SRE engineer — someone who designs infrastructure, not only supports it. I'd like to work more with large-scale cloud platforms and AI infrastructure, and to help other engineers grow, for example by mentoring juniors.

**What would you do if production went down at night?**
First, I would focus on restoring the service as quickly as possible, not on finding the root cause. I'd check the monitoring and the latest changes — very often the problem is a recent deployment, so a rollback is the fastest fix. If I can't solve it quickly, I'd escalate to the right people and keep the team updated in the incident channel. After the service is back, we'd do a blameless postmortem to find the root cause and make sure it doesn't happen again.

**What would you do if you disagreed with your manager's decision?**
I would talk to my manager privately and explain my point of view with facts — for example, risks, costs or data from monitoring. I'd also listen carefully, because the manager may know things I don't. If the decision stays the same, I'd accept it and do my best to make it work, and maybe suggest a way to reduce the risks.

**What would you do if a deadline was unrealistic?**
I would raise it as early as possible, not at the last moment. I'd explain why the deadline is risky and suggest options: reduce the scope and deliver the most important part first, add people, or move the date. Usually it's better to agree on a realistic plan early than to miss the deadline later.

**Are you looking for remote work, and what schedule works for you?**
Draft — adjust it to yourself.

Yes, I'm looking for a remote position. I'm flexible with the schedule, and I'm comfortable working with teams in different time zones — I can overlap [four to six] hours with European or US working hours. I've also been part of on-call rotations before, so that's not a problem for me.

**When can you start?**
Draft — adjust it to yourself.

I can start [right away / within a week], because I'm not bound by a notice period at the moment.

**What are your salary expectations?**
Draft — adjust it to yourself.

Based on my experience and the market for this role, I'm looking for around [amount] USD per month [gross / net]. But I'm open to discussing it — the whole package and the tasks are also important for me.

Tip: name a range or an amount you can justify with your experience; clarify whether it is gross or net (gross — before taxes, net — take-home; for a private-entrepreneur contract the difference is smaller, but it's still worth saying) and what the package includes (vacation, sick leave, insurance, equipment, training). The notice period in the answer above is the time you have to work at your current job after giving notice.

**Do you have any questions for us?**
How long does it usually take to get feedback about the next stage or a final decision?

What is the atmosphere in the company like? Should I worry about layoffs because of AI, for example?

Recommended version of the second question: *"How is the company using AI right now, and how does it affect the team's work?"* Other good questions: *"What does the team look like, and who would I work with most?"*, *"What are the biggest infrastructure challenges right now?"*, *"How is on-call organised?"*, *"What would success look like for me in the first three months?"*
