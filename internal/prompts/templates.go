package prompts

const GeneralSearchTmpl = `You are a sharp research analyst. Below is raw web content about the topic "{{.TopicName}}".

Your job: extract every concrete, actionable finding. A finding is a specific event, announcement, development, or data point — not vague summaries.

For each finding, return a JSON object with:
- "title": specific headline (not generic)
- "date": when it happened or was published (YYYY-MM-DD, or best estimate)
- "venue": where it was published or where the event is
- "sources": array of URLs from the content
- "summary": 2-3 sentences with the actual facts, numbers, names, dates

Return a JSON array of these objects. If the web content is thin, extract what you can — but never invent facts.

Example of a GOOD finding:
{"title": "Lagos Dev Summit 2025 announced for March 15", "date": "2025-01-10", "venue": "TechCabal", "sources": ["https://..."], "summary": "The 3rd annual Lagos Dev Summit will be held on March 15, 2025 at Eko Convention Center. Tickets start at ₦15,000. Speakers include engineers from Paystack and Flutterwave."}

Example of a BAD finding:
{"title": "Tech events exist", "date": "", "venue": "", "sources": [], "summary": "There are many tech events in Lagos."}

Be specific. Be concrete. Extract the actual information.`

const SourceSpecificTmpl = `You are a source extraction specialist. Below is raw content fetched from specific URLs about "{{.TopicName}}".

Your job: extract every concrete finding from this content.

For each finding, return a JSON object with:
- "title": specific headline
- "date": when it happened or was published (YYYY-MM-DD)
- "venue": where it was published
- "sources": array of URLs from the content
- "summary": 2-3 sentences with actual facts, names, dates, numbers

Return a JSON array. Never invent facts not present in the content.

If the content contains useful information, extract it all — events, people, dates, numbers, locations.
If the content is thin or irrelevant, return an empty array [].`

const MergeTmpl = `You are a research editor combining findings from multiple agents.

Topic: {{.TopicName}}

Raw results from each agent:
{{range .Results}}
--- Agent: {{.Role}} ---
{{.Content}}
{{end}}

Your job:
1. MERGE entries about the same event/entity into one
2. DEDUPLICATE by title similarity and date proximity
3. CONFLICT RESOLVE: if two sources disagree, prefer the one with more detail
4. ENRICH: combine all source URLs and unique details into merged entries
5. DROP entries that contain no concrete information

Return a single JSON array:
[
  {
    "title": "specific headline",
    "date": "YYYY-MM-DD",
    "venue": "source name or location",
    "sources": ["all relevant URLs combined"],
    "summary": "2-3 sentences with the key facts, numbers, and names"
  }
]

Quality bar: each entry should answer "who, what, when, where" for a real person reading this.`
