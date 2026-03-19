// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/scalytics/euosint/internal/collector/parse"
)

var (
	technicalSignalPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bcve-\d{4}-\d{4,7}\b`),
		regexp.MustCompile(`(?i)\b(?:ioc|iocs|indicator(?:s)? of compromise)\b`),
		regexp.MustCompile(`(?i)\b(?:tactic|technique|ttp|mitre)\b`),
		regexp.MustCompile(`(?i)\b(?:hash|sha-?256|sha-?1|md5|yara|sigma)\b`),
		regexp.MustCompile(`(?i)\b(?:ip(?:v4|v6)?|domain|url|hostname|command and control|c2)\b`),
		regexp.MustCompile(`(?i)\b(?:vulnerability|exploit(?:ation)?|zero-?day|patch|mitigation|workaround)\b`),
		// German: Schwachstelle (vulnerability), Sicherheitslücke (security flaw),
		// Handlungsempfehlung (recommended action), Warnstufe/Risikostufe (warning/risk level)
		regexp.MustCompile(`(?i)\b(?:schwachstelle|sicherheitsl[uü]cke|handlungsempfehlung|warnstufe|risikostufe|bedrohungslage)\b`),
	}
	incidentDisclosurePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:breach|data leak|compromis(?:e|ed)|intrusion|unauthori[sz]ed access)\b`),
		regexp.MustCompile(`(?i)\b(?:ransomware|malware|botnet|ddos|phishing|credential theft)\b`),
		regexp.MustCompile(`(?i)\b(?:attack|attacked|target(?:ed|ing)|incident response|security incident)\b`),
		regexp.MustCompile(`(?i)\b(?:arrest(?:ed)?|charged|indicted|wanted|fugitive|missing person|kidnapp(?:ed|ing)|homicide)\b`),
		// German incident language
		regexp.MustCompile(`(?i)\b(?:angriff|angegriffen|datenleck|sicherheitsvorfall|erpressung|trojaner|schadsoftware)\b`),
	}
	actionablePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:report|submit (?:a )?tip|contact|hotline|phone|email)\b`),
		regexp.MustCompile(`(?i)\b(?:apply update|upgrade|disable|block|monitor|detect|investigate)\b`),
		regexp.MustCompile(`(?i)\b(?:advisory|alert|warning|incident notice|public appeal)\b`),
		// German: Sicherheitswarnung (security warning), Aktualisierung (update),
		// dringend (urgent), aktualisieren (to update)
		regexp.MustCompile(`(?i)\b(?:sicherheitswarnung|aktualisier(?:ung|en)|dringend|ma[sß]nahme|sofort)\b`),
	}
	narrativePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:opinion|editorial|commentary|analysis|explainer|podcast|interview)\b`),
		regexp.MustCompile(`(?i)\b(?:what we know|live updates|behind the scenes|feature story)\b`),
		regexp.MustCompile(`(?i)\b(?:market reaction|share price|investor)\b`),
	}
	generalNewsPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:announces?|launche[sd]?|conference|summit|webinar|event|awareness month)\b`),
		regexp.MustCompile(`(?i)\b(?:ceremony|speech|statement|newsletter|weekly roundup)\b`),
		regexp.MustCompile(`(?i)\b(?:partnership|memorandum|mou|initiative|campaign)\b`),
	}
	certificationPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:certification|certifi(?:ed|cate)|accreditation|compliance audit|standard(?:s)?)\b`),
		regexp.MustCompile(`(?i)\b(?:NESAS|common criteria|ISO[\s-]?27001|ISO[\s-]?15408|ITSEC|protection profile)\b`),
		regexp.MustCompile(`(?i)\b(?:evaluation|scheme|approval|conformity|audit report|test report)\b`),
		regexp.MustCompile(`(?i)\b(?:product certification|vendor certification|zertifizierung|anerkennung)\b`),
		regexp.MustCompile(`(?i)\b(?:training|course|curriculum|e-learning|online.?training|skill|qualification)\b`),
	}
	securityContextPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:cyber|cybersecurity|infosec|information security|it security)\b`),
		regexp.MustCompile(`(?i)\b(?:security posture|security controls?|threat intelligence)\b`),
		regexp.MustCompile(`(?i)\b(?:vulnerability|exploit|patch|advisory|defend|defensive)\b`),
		regexp.MustCompile(`(?i)\b(?:soc|siem|incident response|malware analysis)\b`),
	}
	// localCrimePatterns match routine domestic police operations that lack
	// cross-border or international intelligence significance.
	localCrimePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:operac[aã]o|opera[çc][aã]o|operation)\b.*\b(?:busca|raid|search|apreens[aã]o|seizure)\b`),
		regexp.MustCompile(`(?i)\b(?:drug bust|drug seizure|narcotics seized|heroin|cocaine|cannabis|marijuana)\b.*\b(?:kg|kilos?|grams?|pounds?|tonnes?)\b`),
		regexp.MustCompile(`(?i)\b(?:burglary|robbery|theft|shoplifting|pickpocket|break-?in|car theft|vehicle theft)\b`),
		regexp.MustCompile(`(?i)\b(?:domestic (?:violence|abuse|dispute)|bar fight|pub brawl|assault|gbh|abh)\b`),
		regexp.MustCompile(`(?i)\b(?:drunk driv|dui|dwi|speeding|traffic (?:offence|offense|violation)|road rage)\b`),
		regexp.MustCompile(`(?i)\b(?:sentenced to|prison sentence|jail (?:term|sentence)|community service|probation order)\b`),
		regexp.MustCompile(`(?i)\b(?:mortu[aá]ri[ao]|autopsy|autópsia|post-?mortem|inquest|coroner)\b`),
		regexp.MustCompile(`(?i)\b(?:local police|polícia local|commissariat|poste de police|comisaría)\b`),
	}
	// crossBorderSignals indicate international/strategic significance that
	// should prevent local-crime downranking.
	crossBorderSignals = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:interpol|europol|eurojust|frontex|five eyes|nato)\b`),
		regexp.MustCompile(`(?i)\b(?:cross-?border|transnational|international|multi-?country|joint (?:operation|investigation))\b`),
		regexp.MustCompile(`(?i)\b(?:terror(?:ism|ist)?|extremis[tm]|radicaliz|foreign fighter)\b`),
		regexp.MustCompile(`(?i)\b(?:cyber.?attack|state-?sponsored|apt|espionage|intelligence)\b`),
		regexp.MustCompile(`(?i)\b(?:trafficking|smuggling|organized crime|money laundering|sanctions evasion)\b`),
		regexp.MustCompile(`(?i)\b(?:critical infrastructure|national security|chemical|biological|nuclear|radiological)\b`),
		regexp.MustCompile(`(?i)\b(?:mass casualty|mass shooting|bombing|explosion|hostage)\b`),
	}
	// actionableTitlePatterns detect titles that carry intelligence or safety
	// value — used to classify noise from broad sources without include_keywords.
	actionableTitlePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:attack(?:ed)?|struck|bomb(?:ed|ing)?|explosion|strike|shelling|shoot(?:ing)?|fire[ds]?\s+(?:on|at|upon))\b`),
		regexp.MustCompile(`(?i)\b(?:seiz(?:ed|ure)|intercept(?:ed)?|captur(?:ed)?|hijack(?:ed)?|board(?:ed|ing))\b`),
		regexp.MustCompile(`(?i)\b(?:missing|wanted|fugitive|kidnapp(?:ed|ing)|abduct(?:ed|ion)|hostage)\b`),
		regexp.MustCompile(`(?i)\b(?:arrest(?:ed)?|charged|indicted|sentenced|convicted|extradited)\b`),
		regexp.MustCompile(`(?i)\b(?:advisory|alert|warning|threat|sanctions?|embargo)\b`),
		regexp.MustCompile(`(?i)\b(?:breach|hack|leak|ransomware|malware|phishing|exploit|vulnerability|cve-\d)\b`),
		regexp.MustCompile(`(?i)\b(?:fraud|scam|theft|launder(?:ing)?|trafficking|smuggling)\b`),
		regexp.MustCompile(`(?i)\b(?:terror(?:ism|ist)?|extremis[tm]|radicaliz)\b`),
		regexp.MustCompile(`(?i)\b(?:killed|dead|death|fatal(?:ity|ities)?|casualt(?:y|ies)|injur(?:ed|y|ies))\b`),
		regexp.MustCompile(`(?i)\b(?:evacuat(?:ed|ion)|disaster|emergency|crisis|outbreak|epidemic|pandemic)\b`),
		regexp.MustCompile(`(?i)\b(?:earthquake|tsunami|flood(?:ing)?|hurricane|typhoon|cyclone|wildfire|eruption)\b`),
		regexp.MustCompile(`(?i)\b(?:piracy|pirate|drone|missile|torpedo|submarine|naval|warship|destroyer|frigate)\b`),
		regexp.MustCompile(`(?i)\b(?:sanction(?:ed|s)?|blacklist(?:ed)?|banned)\b`),
		regexp.MustCompile(`(?i)\bdesignat(?:ed|ion)\b.*\b(?:terrorist|sanction|entity|regime|proliferat)\b`),
		regexp.MustCompile(`(?i)\b(?:ceasefire|truce|peace (?:deal|agreement|talks)|withdrawal|deploy(?:ed|ment))\b`),
		regexp.MustCompile(`(?i)\b(?:espionage|spy|intelligence|surveillance|intercept(?:ion)?)\b`),
		regexp.MustCompile(`(?i)\b(?:coup|uprising|protest|riot|unrest|martial law|state of emergency)\b`),
		regexp.MustCompile(`(?i)\b(?:travel (?:warning|advisory|ban)|do not travel|reisewarnung)\b`),
		regexp.MustCompile(`(?i)\b(?:sunk|sinking|grounding|collision|capsiz(?:ed|ing)|adrift|distress|mayday|sos)\b`),
		regexp.MustCompile(`(?i)\b(?:oil spill|chemical spill|hazmat|contamination)\b`),
		regexp.MustCompile(`(?i)\b(?:nuclear|radiation)\b.*\b(?:incident|accident|emergency|leak|meltdown|fallout|exposure|alert)\b`),
	}
	// informationalTitlePatterns detect titles that are clearly news, PR,
	// or institutional content — conferences, training, partnerships, etc.
	// When matched, these override severity even if a keyword like "nuclear"
	// or "pandemic" appears in the title.
	informationalTitlePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:workshop|seminar|webinar|symposium|colloquium|roundtable)\b`),
		regexp.MustCompile(`(?i)\b(?:training|course|curriculum|e-learning|fellowship|scholarship|capacity.?building)\b`),
		regexp.MustCompile(`(?i)\b(?:conference|congress|forum|summit|convention)\b.*\b(?:held|opens?|convene[sd]?|conclude[sd]?|host(?:ed|s)?|attend|week)\b`),
		regexp.MustCompile(`(?i)\b(?:convene[sd]?|host(?:ed|s)?)\b.*\b(?:week|event|meeting|session|forum)\b`),
		regexp.MustCompile(`(?i)\b(?:collaborat(?:ing|ion)|partner(?:ship|ing)|cooperation|memorandum|mou)\b.*\b(?:centre|center|agreement|signed|framework)\b`),
		regexp.MustCompile(`(?i)\bdesignat(?:ed|ion)\b.*\b(?:collaborat|centre|center|partner|member|focal point)\b`),
		regexp.MustCompile(`(?i)\b(?:review[sd]?|assess(?:es|ed)?|evaluat(?:es|ed)?)\b.*\b(?:infrastructure|development|progress|readiness|framework|programme|program)\b`),
		regexp.MustCompile(`(?i)\b(?:awareness|outreach|education|campaign|initiative|celebration|anniversary|ceremony)\b`),
		regexp.MustCompile(`(?i)\b(?:publication|report release|annual report|yearbook|magazine|newsletter|bulletin)\b`),
		regexp.MustCompile(`(?i)\b(?:experts?|special rapporteurs?)\b.*\b(?:tell|told|urge|urged|call(?:ed)? for|demand(?:s|ed)?)\b.*\b(?:rights?|committee|council)\b`),
		regexp.MustCompile(`(?i)\b(?:weekly|monthly|bi-?weekly|quarterly|daily|annual)\b.*\b(?:report|review|summary|digest|briefing|roundup|round-up|update|recap|overview|bulletin|wrap-up)\b`),
		regexp.MustCompile(`(?i)\b(?:strengthen|bolster|enhance|promote|foster|advance|support)\b.*\b(?:global|regional|national|international)\b.*\b(?:defence|defense|capacity|capability|cooperation|preparedness)\b`),
		regexp.MustCompile(`(?i)\b(?:priorities|strategy|roadmap|vision|principles|objectives|commitments|strategic plan)\b.*\b(?:strengthen|support|protect|trust|confidence|consumer|investor|resilience|compliance|governance)\b`),
		// Regulatory/financial-authority circulars, guidance, consultations.
		regexp.MustCompile(`(?i)\b(?:circular|circulars|guidance|consultation|consultation paper|implementation of|amendments? to|delegated regulation|regulatory technical|technical standards|taxonomy)\b.*\b(?:regulation|directive|market|participants|guidelines|standards|reporting|framework|requirements|esas?ma?|mifid|solvency|ucits|aifmd|priips|sfdr|emir)\b`),
		regexp.MustCompile(`(?i)\b(?:esma|eba|eiopa)\b.*\b(?:report|guidelines|opinion|statement|advice|consultation|peer review|assessment)\b`),
		regexp.MustCompile(`(?i)\b(?:lapsing|withdrawal|surrender|cancellation|revocation)\b.*\b(?:authori[sz]\w*|licen[cs]\w*|registration|permit)\b`),
		regexp.MustCompile(`(?i)\b(?:appoint(?:ed|ment|s)?|elected|nomination|inaugurat)\b`),
	}
	legislativeInformationalTitlePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:experts?|special rapporteurs?)\b.*\b(?:tell|told|urge|urged|call(?:ed)? for|demand(?:s|ed)?)\b.*\b(?:rights?|committee|council)\b`),
		regexp.MustCompile(`(?i)\b(?:committee|council|parliamentary)\b.*\b(?:hears?|debates?|discuss(?:es|ed)?|review(?:s|ed)?)\b`),
		regexp.MustCompile(`(?i)\b(?:statement|remarks|briefing)\b.*\b(?:committee|council|assembly|parliament)\b`),
	}
	assistancePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:report(?:\s+a)?(?:\s+crime)?|submit (?:a )?tip|tip[-\s]?off)\b`),
		regexp.MustCompile(`(?i)\b(?:contact (?:police|authorities|law enforcement)|hotline|helpline)\b`),
		regexp.MustCompile(`(?i)\b(?:if you have information|seeking information|appeal for help)\b`),
		regexp.MustCompile(`(?i)\b(?:missing|wanted|fugitive|amber alert)\b`),
	}
	impactSpecificityPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:affected|impact(?:ed)?|disrupt(?:ed|ion)|outage|service interruption)\b`),
		regexp.MustCompile(`(?i)\b(?:records|accounts|systems|devices|endpoints|victims|organizations)\b`),
		regexp.MustCompile(`(?i)\b(?:on\s+\d{1,2}\s+\w+\s+\d{4}|timeline|between\s+\d{1,2}:\d{2})\b`),
		regexp.MustCompile(`(?i)\b\d{2,}\s+(?:records|users|systems|devices|victims|organizations)\b`),
	}
	newsMediaDomains = []string{
		"channelnewsasia.com",
		"yna.co.kr",
		"nhk.or.jp",
		"scmp.com",
		"jamaicaobserver.com",
		"straitstimes.com",
	}
	newsMediaIDs = map[string]struct{}{
		"cna-sg-crime":     {},
		"yonhap-kr":        {},
		"nhk-jp":           {},
		"scmp-hk":          {},
		"jamaica-observer": {},
		"straitstimes-sg":  {},
	}
	blogFilterExempt = map[string]struct{}{
		"bleepingcomputer": {},
		"krebsonsecurity":  {},
		"thehackernews":    {},
		"databreaches-net": {},
		"cbc-canada":       {},
		"globalnews-ca":    {},
	}
)

type Context struct {
	Config   config.Config
	Now      time.Time
	Geocoder *Geocoder // optional; nil falls back to country-level only
}

type FeedContext struct {
	Summary  string
	Author   string
	Tags     []string
	FeedType string
}

func RSSItem(ctx Context, meta model.RegistrySource, item parse.FeedItem) *model.Alert {
	publishedAt := parseDate(item.Published)
	if publishedAt.IsZero() {
		publishedAt = ctx.Now
	}
	if !isFresh(ctx.Config, publishedAt, ctx.Now) {
		return nil
	}
	alert := baseAlert(ctx, meta, item.Title, item.Link, item.Title+"\n"+item.Summary, publishedAt)
	// Feed-provided coordinates (e.g. <georss:point>) override geocoding.
	if item.Lat != 0 || item.Lng != 0 {
		alert.Lat, alert.Lng = jitter(item.Lat, item.Lng, meta.Source.SourceID+":"+item.Link, "georss")
		// Resolve country from text for display, but keep feed coords.
		if _, _, code, ok := geocodeText(item.Title + " " + item.Summary); ok {
			if name := countryNameFromCode(code); name != "" {
				alert.Source.Country = name
				alert.Source.CountryCode = code
			}
		}
	}
	triage := score(ctx.Config, alert, FeedContext{
		Summary:  item.Summary,
		Author:   item.Author,
		Tags:     item.Tags,
		FeedType: meta.Type,
	})
	alert.Triage = triage
	alert = normalizeInformational(ctx.Config, alert, FeedContext{
		Summary:  item.Summary,
		Author:   item.Author,
		Tags:     item.Tags,
		FeedType: meta.Type,
	})
	assignSubcategory(&alert, item.Summary, item.Author, strings.Join(item.Tags, " "))
	return &alert
}

func HTMLItem(ctx Context, meta model.RegistrySource, item parse.FeedItem) *model.Alert {
	alert := baseAlert(ctx, meta, item.Title, item.Link, item.Title+"\n"+item.Summary, ctx.Now)
	triage := score(ctx.Config, alert, FeedContext{
		Summary:  item.Summary,
		Tags:     item.Tags,
		FeedType: meta.Type,
	})
	alert.Triage = triage
	alert = normalizeInformational(ctx.Config, alert, FeedContext{
		Summary:  item.Summary,
		Tags:     item.Tags,
		FeedType: meta.Type,
	})
	assignSubcategory(&alert, item.Summary, strings.Join(item.Tags, " "))
	return &alert
}

func KEVAlert(ctx Context, meta model.RegistrySource, cveID string, vulnName string, description string, dateAdded string, knownRansomware bool) *model.Alert {
	publishedAt := parseDate(dateAdded)
	if publishedAt.IsZero() || !isFresh(ctx.Config, publishedAt, ctx.Now) {
		return nil
	}
	title := cveID + ": " + firstNonEmpty(vulnName, "Known Exploited Vulnerability")
	link := meta.Source.BaseURL
	if strings.TrimSpace(cveID) != "" {
		link = "https://nvd.nist.gov/vuln/detail/" + strings.TrimSpace(cveID)
	}
	alert := baseAlert(ctx, meta, title, link, title+"\n"+description, publishedAt)
	if hoursBetween(ctx.Now, publishedAt) <= 72 {
		alert.Severity = "critical"
	} else if hoursBetween(ctx.Now, publishedAt) <= 168 {
		alert.Severity = "high"
	}
	tags := []string{}
	if knownRansomware {
		tags = append(tags, "known-ransomware-campaign")
	}
	alert.Triage = score(ctx.Config, alert, FeedContext{
		Summary:  strings.TrimSpace(vulnName + " " + description),
		Tags:     tags,
		FeedType: meta.Type,
	})
	assignSubcategory(&alert, vulnName, description, strings.Join(tags, " "))
	return &alert
}

func InterpolAlert(ctx Context, meta model.RegistrySource, noticeID string, title string, link string, countryCode string, summary string, tags []string) *model.Alert {
	if strings.TrimSpace(title) == "" {
		return nil
	}
	alert := baseAlert(ctx, meta, title, firstNonEmpty(link, meta.Source.BaseURL), title+"\n"+summary, ctx.Now)
	alert.Severity = "critical"
	if id := strings.TrimSpace(noticeID); id != "" {
		alert.AlertID = meta.Source.SourceID + ":" + id
	}
	if code := normalizeCountryCode(countryCode); code != "" {
		alert.RegionTag = code
		alert.Source.CountryCode = code
		if name := countryNameFromCode(code); name != "" {
			alert.Source.Country = name
			alert.EventCountry = name
		}
		alert.EventCountryCode = code
		// Override lat/lng to the person's nationality country instead of
		// Interpol HQ (Lyon, France).
		if gLat, gLng, _, ok := geocodeCountryCode(code); ok {
			alert.Lat, alert.Lng = jitterCC(gLat, gLng, meta.Source.SourceID+":"+link, "capital", code)
			alert.EventGeoSource = "capital"
			alert.EventGeoConfidence = eventGeoConfidence("capital")
		}
	}
	alert.Triage = score(ctx.Config, alert, FeedContext{
		Summary:  summary,
		Tags:     tags,
		FeedType: meta.Type,
	})
	assignSubcategory(&alert, summary, strings.Join(tags, " "))
	return &alert
}

func FBIWantedAlert(ctx Context, meta model.RegistrySource, item parse.FeedItem) *model.Alert {
	publishedAt := parseDate(item.Published)
	if publishedAt.IsZero() {
		publishedAt = ctx.Now
	}
	alert := baseAlert(ctx, meta, item.Title, item.Link, item.Title+"\n"+item.Summary, publishedAt)
	alert.Severity = "critical"
	triage := score(ctx.Config, alert, FeedContext{
		Summary:  item.Summary,
		Tags:     item.Tags,
		FeedType: meta.Type,
	})
	alert.Triage = triage
	assignSubcategory(&alert, item.Summary, strings.Join(item.Tags, " "))
	return &alert
}

func ACLEDAlert(ctx Context, meta model.RegistrySource, ev parse.ACLEDItem) *model.Alert {
	publishedAt := parseDate(ev.Published)
	if publishedAt.IsZero() {
		publishedAt = ctx.Now
	}
	if !isFresh(ctx.Config, publishedAt, ctx.Now) {
		return nil
	}
	alert := baseAlert(ctx, meta, ev.Title, ev.Link, ev.Title+"\n"+ev.Summary, publishedAt)
	alert.Category = parse.ACLEDEventCategory(ev.EventType)
	alert.Severity = parse.ACLEDEventSeverity(ev.EventType, ev.Fatalities)
	// Use ACLED's precise coordinates instead of registry defaults.
	if ev.Lat != 0 || ev.Lng != 0 {
		alert.Lat = ev.Lat
		alert.Lng = ev.Lng
	}
	// Override source metadata with per-event country from ACLED.
	if iso2 := parse.ACLEDISO2(ev.ISO3); iso2 != "" {
		alert.Source.CountryCode = iso2
		alert.Source.Country = ev.Country
		alert.RegionTag = iso2
		alert.EventCountry = ev.Country
		alert.EventCountryCode = iso2
		alert.EventGeoSource = "coordinates"
		alert.EventGeoConfidence = eventGeoConfidence("coordinates")
	}
	if ev.Region != "" {
		alert.Source.Region = ev.Region
	}
	triage := score(ctx.Config, alert, FeedContext{
		Summary:  ev.Summary,
		Tags:     ev.Tags,
		FeedType: meta.Type,
	})
	alert.Triage = triage
	assignSubcategory(&alert, ev.Summary, ev.EventType, strings.Join(ev.Tags, " "))
	return &alert
}

func UCDPAlert(ctx Context, meta model.RegistrySource, ev parse.UCDPItem) *model.Alert {
	publishedAt := parseDate(ev.Published)
	if publishedAt.IsZero() {
		publishedAt = ctx.Now
	}
	if !isFresh(ctx.Config, publishedAt, ctx.Now) {
		return nil
	}
	alert := baseAlert(ctx, meta, ev.Title, ev.Link, ev.Title+"\n"+ev.Summary, publishedAt)
	alert.Category = "conflict_monitoring"
	alert.Severity = "medium"
	if ev.Fatalities >= 25 {
		alert.Severity = "critical"
	} else if ev.Fatalities > 0 {
		alert.Severity = "high"
	}
	if ev.Lat != 0 || ev.Lng != 0 {
		alert.Lat = ev.Lat
		alert.Lng = ev.Lng
		alert.EventGeoSource = "coordinates"
		alert.EventGeoConfidence = eventGeoConfidence("coordinates")
	}
	if name := strings.TrimSpace(ev.Country); name != "" {
		alert.EventCountry = name
		alert.Source.Country = name
		if _, _, code, ok := geocodeText(name); ok {
			alert.EventCountryCode = code
			alert.Source.CountryCode = code
			alert.RegionTag = code
		}
	}
	if region := strings.TrimSpace(ev.Region); region != "" {
		alert.Source.Region = region
	}
	triage := score(ctx.Config, alert, FeedContext{
		Summary:  ev.Summary,
		Tags:     ev.Tags,
		FeedType: meta.Type,
	})
	alert.Triage = triage
	assignSubcategory(&alert, ev.Summary, ev.Region, strings.Join(ev.Tags, " "))
	return &alert
}

func TravelWarningAlert(ctx Context, meta model.RegistrySource, item parse.FeedItem) *model.Alert {
	publishedAt := parseDate(item.Published)
	if publishedAt.IsZero() {
		publishedAt = ctx.Now
	}
	if !isFresh(ctx.Config, publishedAt, ctx.Now) {
		return nil
	}
	alert := baseAlert(ctx, meta, item.Title, item.Link, item.Title+"\n"+item.Summary, publishedAt)
	alert.Severity = inferTravelWarningSeverity(item.Title, item.Summary, item.Tags)
	triage := score(ctx.Config, alert, FeedContext{
		Summary:  item.Summary,
		Author:   item.Author,
		Tags:     item.Tags,
		FeedType: meta.Type,
	})
	alert.Triage = triage
	assignSubcategory(&alert, item.Summary, item.Author, strings.Join(item.Tags, " "))
	return &alert
}

func inferTravelWarningSeverity(title, summary string, tags []string) string {
	text := strings.ToLower(title + " " + summary + " " + strings.Join(tags, " "))
	switch {
	case containsAny(text, "do not travel", "reisewarnung", "advise against all travel", "level 4"):
		return "critical"
	case containsAny(text, "reconsider travel", "avoid non-essential travel", "advise against all but essential travel", "level 3", "teilreisewarnung"):
		return "high"
	case containsAny(text, "exercise increased caution", "exercise a high degree of caution", "level 2"):
		return "medium"
	default:
		return "medium"
	}
}

func USGSAlert(ctx Context, meta model.RegistrySource, ev parse.USGSItem) *model.Alert {
	publishedAt := parseDate(ev.Published)
	if publishedAt.IsZero() {
		publishedAt = ctx.Now
	}
	if !isFresh(ctx.Config, publishedAt, ctx.Now) {
		return nil
	}
	alert := baseAlert(ctx, meta, ev.Title, ev.Link, ev.Title+"\n"+ev.Summary, publishedAt)
	alert.Category = "public_safety"
	alert.Severity = parse.USGSSeverity(ev.Magnitude, ev.AlertLevel)
	// Use GeoJSON coordinates directly (precise epicenter).
	if ev.Lat != 0 || ev.Lng != 0 {
		alert.Lat = ev.Lat
		alert.Lng = ev.Lng
	}
	triage := score(ctx.Config, alert, FeedContext{
		Summary:  ev.Summary,
		Tags:     ev.Tags,
		FeedType: meta.Type,
	})
	alert.Triage = triage
	assignSubcategory(&alert, ev.Summary, strings.Join(ev.Tags, " "))
	return &alert
}

func EONETAlert(ctx Context, meta model.RegistrySource, ev parse.EONETItem) *model.Alert {
	publishedAt := parseDate(ev.Published)
	if publishedAt.IsZero() {
		publishedAt = ctx.Now
	}
	if !isFresh(ctx.Config, publishedAt, ctx.Now) {
		return nil
	}
	alert := baseAlert(ctx, meta, ev.Title, ev.Link, ev.Title+"\n"+ev.Summary, publishedAt)
	alert.Category = "public_safety"
	alert.Severity = parse.EONETSeverity(ev.CategoryID)
	if ev.Lat != 0 || ev.Lng != 0 {
		alert.Lat = ev.Lat
		alert.Lng = ev.Lng
	}
	triage := score(ctx.Config, alert, FeedContext{
		Summary:  ev.Summary,
		Tags:     ev.Tags,
		FeedType: meta.Type,
	})
	alert.Triage = triage
	assignSubcategory(&alert, ev.Summary, strings.Join(ev.Tags, " "))
	return &alert
}

func GDELTAlert(ctx Context, meta model.RegistrySource, item parse.FeedItem, sourceCountry string) *model.Alert {
	publishedAt := parseDate(item.Published)
	if publishedAt.IsZero() {
		publishedAt = ctx.Now
	}
	if !isFresh(ctx.Config, publishedAt, ctx.Now) {
		return nil
	}
	alert := baseAlert(ctx, meta, item.Title, item.Link, item.Title+"\n"+item.Summary, publishedAt)
	// Pin to country capital when we have a source country.
	if iso2 := parse.GDELTCountryISO2(sourceCountry); iso2 != "" {
		if gLat, gLng, _, ok := geocodeCountryCode(iso2); ok {
			alert.Lat, alert.Lng = jitterCC(gLat, gLng, meta.Source.SourceID+":"+item.Link, "capital", iso2)
			alert.EventGeoSource = "capital"
			alert.EventGeoConfidence = eventGeoConfidence("capital")
		}
		alert.Source.CountryCode = iso2
		if name := countryNameFromCode(iso2); name != "" {
			alert.Source.Country = name
			alert.EventCountry = name
		}
		alert.EventCountryCode = iso2
		alert.RegionTag = iso2
	}
	triage := score(ctx.Config, alert, FeedContext{
		Summary:  item.Summary,
		Author:   item.Author,
		Tags:     item.Tags,
		FeedType: meta.Type,
	})
	alert.Triage = triage
	assignSubcategory(&alert, item.Summary, item.Author, sourceCountry, strings.Join(item.Tags, " "))
	return &alert
}

func FeodoAlert(ctx Context, meta model.RegistrySource, ev parse.FeodoItem) *model.Alert {
	publishedAt := parseDate(ev.Published)
	if publishedAt.IsZero() {
		publishedAt = ctx.Now
	}
	if !isFresh(ctx.Config, publishedAt, ctx.Now) {
		return nil
	}
	alert := baseAlert(ctx, meta, ev.Title, ev.Link, ev.Title+"\n"+ev.Summary, publishedAt)
	alert.Category = "cyber_advisory"
	alert.Severity = "high"
	// Pin to country capital using the 2-letter country code from Feodo.
	if code := normalizeCountryCode(ev.Country); code != "" {
		if gLat, gLng, _, ok := geocodeCountryCode(code); ok {
			alert.Lat, alert.Lng = jitterCC(gLat, gLng, meta.Source.SourceID+":"+ev.IPAddress, "capital", code)
			alert.EventGeoSource = "capital"
			alert.EventGeoConfidence = eventGeoConfidence("capital")
		}
		alert.Source.CountryCode = code
		if name := countryNameFromCode(code); name != "" {
			alert.Source.Country = name
			alert.EventCountry = name
		}
		alert.EventCountryCode = code
		alert.RegionTag = code
	}
	triage := score(ctx.Config, alert, FeedContext{
		Summary:  ev.Summary,
		Tags:     ev.Tags,
		FeedType: meta.Type,
	})
	alert.Triage = triage
	assignSubcategory(&alert, ev.Summary, ev.IPAddress, strings.Join(ev.Tags, " "))
	return &alert
}

func StaticInterpolEntry(now time.Time) model.Alert {
	return model.Alert{
		AlertID:            "interpol-hub-static",
		SourceID:           "interpol-hub",
		Source:             model.SourceMetadata{SourceID: "interpol-hub", AuthorityName: "INTERPOL Notices Hub", Country: "France", CountryCode: "FR", Region: "International", AuthorityType: "police", BaseURL: "https://www.interpol.int"},
		Title:              "INTERPOL Red & Yellow Notices - Browse Wanted & Missing Persons",
		CanonicalURL:       "https://www.interpol.int/How-we-work/Notices/View-Red-Notices",
		FirstSeen:          now.UTC().Format(time.RFC3339),
		LastSeen:           now.UTC().Format(time.RFC3339),
		Status:             "active",
		Category:           "wanted_suspect",
		Subcategory:        "wanted_notice",
		Severity:           "critical",
		SignalLane:         model.SignalLaneAlarm,
		RegionTag:          "INT",
		Lat:                45.764,
		Lng:                4.8357,
		EventCountry:       "France",
		EventCountryCode:   "FR",
		EventGeoSource:     "capital",
		EventGeoConfidence: eventGeoConfidence("capital"),
		FreshnessHours:     1,
		Reporting: model.ReportingMetadata{
			Label: "Browse INTERPOL Notices",
			URL:   "https://www.interpol.int/How-we-work/Notices/View-Red-Notices",
			Notes: "Red Notices: wanted persons. Yellow Notices: missing persons. Browse directly.",
		},
		Triage: &model.Triage{RelevanceScore: 1, Reasoning: "Permanent INTERPOL hub link"},
	}
}

func baseAlert(ctx Context, meta model.RegistrySource, title string, link string, geoText string, publishedAt time.Time) model.Alert {
	title = strings.TrimSpace(title)
	geoText = strings.TrimSpace(geoText)
	if geoText == "" {
		geoText = title
	}
	geoText = sanitizeGeoText(geoText)
	// Fix broken NCMEC-style titles that start with ": Name (State)".
	if strings.HasPrefix(title, ": ") {
		title = "Missing" + title
	}

	baseLat, baseLng := meta.Lat, meta.Lng
	geoSource := "registry"
	source := meta.Source

	// Use capital coords instead of geographic centroid for the source's
	// country — fixes islands (Malta, Cyprus, etc.) landing in the sea.
	if source.CountryCode != "" && source.CountryCode != "INT" {
		if capital, ok := capitalCoords[source.CountryCode]; ok {
			baseLat, baseLng = capital[0], capital[1]
			geoSource = "capital"
		}
	}

	geocoded := false
	allowDynamicGeocode := shouldUseDynamicGeocoding(meta.Category)

	// For international sources and travel warnings, geocode to the actual
	// target location instead of pinning to the issuing org's HQ.
	isCrossCountry := meta.RegionTag == "INT" || meta.Source.CountryCode == "INT" || meta.Category == "travel_warning"
	if isLikelyNewsAggregator(meta) {
		isCrossCountry = true
	}
	skipLLMGeo := isPersonCategory(meta.Category)
	if allowDynamicGeocode && isCrossCountry {
		if ctx.Geocoder != nil {
			// Enhanced geocoding: city DB → Nominatim → country text.
			// Skip LLM fallback for person-centric categories (Interpol, FBI)
			// where text describes people, not places.
			var result GeoResult
			if skipLLMGeo {
				result = ctx.Geocoder.ResolveWithoutLLM(context.Background(), geoText, "")
			} else {
				result = ctx.Geocoder.Resolve(context.Background(), geoText, "")
			}
			if result.CountryCode != "" {
				baseLat, baseLng = result.Lat, result.Lng
				geoSource = result.Source
				geocoded = true
				if name := countryNameFromCode(result.CountryCode); name != "" {
					source.Country = name
					source.CountryCode = result.CountryCode
				}
			}
		}
		if !geocoded {
			if _, _, code, ok := geocodeText(geoText); ok {
				// Use capital coords instead of centroid to avoid water/desert.
				if capital, cok := capitalCoords[code]; cok {
					baseLat, baseLng = capital[0], capital[1]
				}
				geoSource = "capital"
				geocoded = true
				if name := countryNameFromCode(code); name != "" {
					source.Country = name
					source.CountryCode = code
				}
			}
		}
		if !geocoded {
			// No location resolved from the alert text — zero out coords
			// so the map doesn't show a misleading pin at the source's HQ
			// (e.g. Athens for Hellenic Shipping, NYC for UN sources).
			baseLat, baseLng = 0, 0
			geoSource = ""
		}
	} else if allowDynamicGeocode && ctx.Geocoder != nil {
		// Non-international source: try city-level geocoding within the
		// source's country for better pin placement. Only accept results
		// that match the source's country to prevent cross-country false
		// positives (e.g. Malta news pinned to Brazil).
		var result GeoResult
		if skipLLMGeo {
			result = ctx.Geocoder.ResolveWithoutLLM(context.Background(), geoText, source.CountryCode)
		} else {
			result = ctx.Geocoder.Resolve(context.Background(), geoText, source.CountryCode)
		}
		if result.CountryCode != "" &&
			result.CountryCode == source.CountryCode {
			baseLat, baseLng = result.Lat, result.Lng
			geoSource = result.Source
		}
	}

	lat, lng := jitterCC(baseLat, baseLng, meta.Source.SourceID+":"+link, geoSource, source.CountryCode)
	eventCountry := source.Country
	eventCountryCode := source.CountryCode
	if eventCountryCode == "INT" {
		eventCountry = ""
		eventCountryCode = ""
	}
	canonicalCategory := canonicalCategory(meta.Category)
	canonicalSeverity := inferSeverity(title, defaultSeverity(canonicalCategory))
	if canonicalCategory == "informational" {
		canonicalSeverity = "info"
	}
	return model.Alert{
		AlertID:            meta.Source.SourceID + "-" + hashID(link),
		SourceID:           meta.Source.SourceID,
		Source:             source,
		Title:              strings.TrimSpace(title),
		CanonicalURL:       strings.TrimSpace(link),
		FirstSeen:          publishedAt.UTC().Format(time.RFC3339),
		LastSeen:           ctx.Now.UTC().Format(time.RFC3339),
		Status:             "active",
		Category:           canonicalCategory,
		Subcategory:        inferSubcategory(canonicalCategory, title+"\n"+geoText),
		Severity:           canonicalSeverity,
		RegionTag:          meta.RegionTag,
		Lat:                lat,
		Lng:                lng,
		EventCountry:       eventCountry,
		EventCountryCode:   eventCountryCode,
		EventGeoSource:     geoSource,
		EventGeoConfidence: eventGeoConfidence(geoSource),
		FreshnessHours:     hoursBetween(ctx.Now, publishedAt),
		Reporting:          meta.Reporting,
	}
}

func canonicalCategory(category string) string {
	c := strings.ToLower(strings.TrimSpace(category))
	switch c {
	case "public_appeal":
		return "public_safety"
	case "intelligence_report", "private_sector", "education_digital_capacity", "humanitarian_tasking":
		return "informational"
	case "humanitarian_security":
		return "conflict_monitoring"
	case "emergency_management":
		return "public_safety"
	default:
		return c
	}
}

func sanitizeGeoText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	// Remove trailing publisher attributions that poison geocoding
	// (e.g. "... - The New York Times" => false York/UK matches).
	for _, sep := range []string{" - ", " | ", " — ", " – "} {
		parts := strings.Split(text, sep)
		if len(parts) < 2 {
			continue
		}
		suffix := strings.ToLower(strings.TrimSpace(parts[len(parts)-1]))
		if looksLikePublisherSuffix(suffix) {
			return strings.TrimSpace(strings.Join(parts[:len(parts)-1], sep))
		}
	}
	return text
}

func looksLikePublisherSuffix(s string) bool {
	if s == "" {
		return false
	}
	publisherHints := []string{
		"new york times", "nytimes", "reuters", "associated press", "ap news",
		"bbc", "cnn", "guardian", "washington post", "financial times",
		"bloomberg", "al jazeera", "the times", "fox news", "nbc news",
	}
	for _, hint := range publisherHints {
		if strings.Contains(s, hint) {
			return true
		}
	}
	// Generic "xxx news" fallback.
	return strings.HasSuffix(s, " news")
}

func isLikelyNewsAggregator(meta model.RegistrySource) bool {
	authorityType := strings.ToLower(strings.TrimSpace(meta.Source.AuthorityType))
	if strings.Contains(authorityType, "news") || authorityType == "media" {
		return true
	}
	if _, ok := newsMediaIDs[strings.ToLower(strings.TrimSpace(meta.Source.SourceID))]; ok {
		return true
	}
	base := strings.ToLower(strings.TrimSpace(meta.Source.BaseURL))
	for _, domain := range newsMediaDomains {
		if strings.Contains(base, domain) {
			return true
		}
	}
	return false
}

// isPersonCategory returns true for categories where alert text describes
// a person (name, nationality, charges) rather than a geographic event.
// The LLM geo tier is skipped for these — it can't extract location from
// "John DOE wanted for fraud" and wastes API calls trying.
func isPersonCategory(category string) bool {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "wanted_suspect", "missing_person", "public_appeal":
		return true
	default:
		return false
	}
}

func shouldUseDynamicGeocoding(category string) bool {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "missing_person", "wanted_suspect", "public_appeal", "public_safety",
		"travel_warning", "humanitarian_tasking", "humanitarian_security",
		"conflict_monitoring", "maritime_security", "logistics_incident",
		"health_emergency", "disease_outbreak", "environmental_disaster",
		"emergency_management":
		return true
	default:
		return false
	}
}

func normalizeCountryCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) == 2 {
		return code
	}
	return ""
}

func countryNameFromCode(code string) string {
	code = normalizeCountryCode(code)
	// Use the geocode table as the canonical source.
	for i := range geoCountries {
		if geoCountries[i].Code == code {
			return geoCountries[i].Name
		}
	}
	return ""
}

func score(cfg config.Config, alert model.Alert, feed FeedContext) *model.Triage {
	text := strings.ToLower(strings.Join([]string{
		alert.Title,
		feed.Summary,
		feed.Author,
		strings.Join(feed.Tags, " "),
		alert.CanonicalURL,
	}, "\n"))
	publicationType := inferPublicationType(alert, feed.FeedType)
	score := 0.5
	signals := []string{}
	add := func(delta float64, reason string) {
		score += delta
		if delta >= 0 {
			signals = append(signals, "+"+formatDelta(delta)+" "+reason)
			return
		}
		signals = append(signals, formatDelta(delta)+" "+reason)
	}

	switch publicationType {
	case "news_media":
		add(-0.16, "publication type leans general-news")
	case "cert_advisory", "structured_incident_feed":
		add(0.08, "source metadata is incident-oriented")
	case "law_enforcement":
		add(0.06, "law-enforcement source metadata")
	}

	switch alert.Category {
	case "cyber_advisory":
		add(0.09, "cyber advisory category")
	case "wanted_suspect", "missing_person":
		add(0.09, "law-enforcement incident category")
	case "humanitarian_tasking", "conflict_monitoring", "humanitarian_security":
		add(0.08, "humanitarian incident/tasking category")
	case "maritime_security":
		add(0.08, "maritime security category")
	case "logistics_incident":
		add(0.08, "logistics/transportation incident category")
	case "education_digital_capacity":
		add(0.07, "education and digital capacity category")
	case "fraud_alert":
		add(0.07, "fraud incident category")
	case "travel_warning":
		add(0.08, "travel warning category")
	}

	hasTechnical := hasAny(text, technicalSignalPatterns)
	hasIncident := hasAny(text, incidentDisclosurePatterns)
	hasActionable := hasAny(text, actionablePatterns)
	hasSpecificImpact := hasAny(text, impactSpecificityPatterns)
	hasNarrative := hasAny(text, narrativePatterns)
	hasGeneral := hasAny(text, generalNewsPatterns)
	hasCertification := hasAny(text, certificationPatterns)
	looksLikeBlog := isBlog(alert)

	if hasTechnical {
		add(0.16, "technical indicators or tactics present")
	}
	if hasIncident {
		add(0.16, "incident/crime disclosure language")
	}
	if hasActionable {
		add(0.10, "contains response/reporting actions")
	}
	if hasSpecificImpact {
		add(0.08, "specific impact/timeline/system details")
	}
	if hasNarrative {
		add(-0.18, "opinion/commentary phrasing")
	}
	if hasGeneral {
		add(-0.12, "general institutional/news language")
	}
	if looksLikeBlog {
		add(-0.10, "blog-style structure")
	}
	if hasCertification && !hasIncident && !hasTechnical {
		add(-0.22, "certification/training/standards content")
	}
	// Downrank routine local crime stories from police feeds unless
	// they carry cross-border or strategic intelligence significance.
	hasLocalCrime := hasAny(text, localCrimePatterns)
	hasCrossBorder := hasAny(text, crossBorderSignals)
	if hasLocalCrime && !hasCrossBorder && !hasTechnical {
		add(-0.20, "routine local crime without cross-border significance")
	}
	if !hasTechnical && !hasIncident && (hasNarrative || hasGeneral) {
		add(-0.08, "weak incident evidence relative to narrative cues")
	}
	if alert.FreshnessHours > 0 && alert.FreshnessHours <= 24 && (hasIncident || hasTechnical) {
		add(0.04, "fresh post with potential early-warning signal")
	}
	if fusion := threatFusionMatchCount(text); fusion >= 2 {
		add(0.14, "fraud/laundering/organized-crime/terror co-occurrence")
		if fusion >= 3 {
			add(0.06, "multi-domain criminal-threat fusion")
		}
	}

	threshold := clamp01(cfg.IncidentRelevanceThreshold)
	relevance := round3(clamp01(score))
	distance := math.Abs(relevance - threshold)
	confidence := "low"
	if distance >= 0.25 {
		confidence = "high"
	} else if distance >= 0.1 {
		confidence = "medium"
	}
	disposition := "filtered_review"
	if relevance >= threshold {
		disposition = "retained"
	}
	return &model.Triage{
		RelevanceScore:  relevance,
		Threshold:       threshold,
		Confidence:      confidence,
		Disposition:     disposition,
		PublicationType: publicationType,
		WeakSignals:     limitStrings(signals, 12),
		Metadata: &model.TriageMetadata{
			Author: strings.TrimSpace(feed.Author),
			Tags:   limitStrings(feed.Tags, 8),
		},
	}
}

func normalizeInformational(cfg config.Config, alert model.Alert, feed FeedContext) model.Alert {
	alert.Category = canonicalCategory(alert.Category)
	if alert.Category == "informational" {
		alert.Severity = "info"
	}
	if shouldDowngradeLegislativeToInformational(alert, feed) {
		threshold := clamp01(cfg.IncidentRelevanceThreshold)
		if alert.Triage != nil {
			score := math.Max(alert.Triage.RelevanceScore, threshold)
			alert.Triage.RelevanceScore = round3(score)
			alert.Triage.Threshold = threshold
			alert.Triage.Confidence = "medium"
			alert.Triage.Disposition = "retained"
			alert.Triage.WeakSignals = append([]string{"reclassified as informational institutional statement"}, limitStrings(alert.Triage.WeakSignals, 10)...)
		}
		alert.Category = "informational"
		alert.Severity = "info"
		assignSubcategory(&alert, feed.Summary, feed.Author, strings.Join(feed.Tags, " "))
		return alert
	}
	if !isSecurityInformational(alert, feed) || alert.Triage == nil {
		assignSubcategory(&alert, feed.Summary, feed.Author, strings.Join(feed.Tags, " "))
		return alert
	}
	threshold := clamp01(cfg.IncidentRelevanceThreshold)
	score := math.Max(alert.Triage.RelevanceScore, threshold)
	alert.Category = "informational"
	alert.Severity = "info"
	alert.Triage.RelevanceScore = round3(score)
	alert.Triage.Threshold = threshold
	alert.Triage.Confidence = "medium"
	alert.Triage.Disposition = "retained"
	alert.Triage.WeakSignals = append([]string{"reclassified as informational security/cybersecurity update"}, limitStrings(alert.Triage.WeakSignals, 10)...)
	assignSubcategory(&alert, feed.Summary, feed.Author, strings.Join(feed.Tags, " "))
	return alert
}

func assignSubcategory(alert *model.Alert, parts ...string) {
	if alert == nil {
		return
	}
	text := alert.Title
	if len(parts) > 0 {
		text += "\n" + strings.Join(parts, "\n")
	}
	alert.Subcategory = inferSubcategory(alert.Category, text)
}

func inferSubcategory(category string, text string) string {
	cat := strings.ToLower(strings.TrimSpace(category))
	lower := strings.ToLower(strings.TrimSpace(text))
	if cat == "" || lower == "" {
		return ""
	}
	switch cat {
	case "cyber_advisory":
		switch {
		case containsAny(lower, "cve-", "vulnerability", "zero-day", "patch"):
			return "vulnerability"
		case containsAny(lower, "ransomware", "extortion"):
			return "ransomware"
		case containsAny(lower, "phishing", "smishing", "vishing"):
			return "phishing"
		case containsAny(lower, "breach", "data leak", "compromised", "unauthorized access"):
			return "data_breach"
		default:
			return "cyber_incident"
		}
	case "conflict_monitoring":
		switch {
		case containsAny(lower, "airstrike", "air strike", "missile", "drone strike"):
			return "airstrike"
		case containsAny(lower, "shelling", "artillery", "ground assault", "clashes", "clash"):
			return "ground_clash"
		case containsAny(lower, "ceasefire", "truce", "peace talk", "peace talks"):
			return "ceasefire"
		default:
			return "conflict_event"
		}
	case "maritime_security":
		switch {
		case containsAny(lower, "piracy", "pirate", "hijack", "boarded", "boarding"):
			return "piracy"
		case containsAny(lower, "collision", "grounding", "capsized", "distress", "sos", "mayday"):
			return "vessel_incident"
		case containsAny(lower, "port closed", "port closure", "port disruption", "canal"):
			return "port_disruption"
		default:
			return "maritime_event"
		}
	case "logistics_incident":
		switch {
		case containsAny(lower, "port closed", "port closure", "port disruption", "terminal"):
			return "port_disruption"
		case containsAny(lower, "rail", "train derail", "derailment", "track closure"):
			return "rail_disruption"
		case containsAny(lower, "airport", "runway", "flight suspension", "airspace"):
			return "aviation_disruption"
		default:
			return "supply_chain_disruption"
		}
	case "environmental_disaster":
		switch {
		case containsAny(lower, "earthquake", "aftershock", "seismic"):
			return "earthquake"
		case containsAny(lower, "eruption", "volcano", "lava"):
			return "volcanic_activity"
		case containsAny(lower, "flood", "flooding", "inundation"):
			return "flood"
		case containsAny(lower, "wildfire", "forest fire", "bushfire"):
			return "wildfire"
		case containsAny(lower, "cyclone", "hurricane", "typhoon", "storm"):
			return "storm"
		default:
			return "natural_hazard"
		}
	case "disease_outbreak", "health_emergency":
		if containsAny(lower, "outbreak", "cluster", "epidemic", "pandemic") {
			return "outbreak"
		}
		return "public_health"
	case "fraud_alert":
		switch {
		case containsAny(lower, "money laundering", "laundering"):
			return "money_laundering"
		case containsAny(lower, "organized crime", "trafficking", "cartel"):
			return "organized_crime"
		default:
			return "fraud_scam"
		}
	case "terrorism_tip":
		if containsAny(lower, "financing", "funding", "material support") {
			return "terror_financing"
		}
		return "terror_activity"
	case "travel_warning":
		switch {
		case containsAny(lower, "level 4", "do not travel", "advise against all travel"):
			return "advisory_level_4"
		case containsAny(lower, "level 3", "reconsider travel", "all but essential travel"):
			return "advisory_level_3"
		case containsAny(lower, "level 2", "exercise increased caution"):
			return "advisory_level_2"
		default:
			return "travel_advisory"
		}
	case "wanted_suspect":
		return "wanted_notice"
	case "missing_person":
		return "missing_notice"
	case "public_safety":
		switch {
		case containsAny(lower, "earthquake", "eruption", "wildfire", "flood", "storm"):
			return "hazard_warning"
		case containsAny(lower, "arrest", "detained", "seized", "raid"):
			return "law_enforcement_action"
		default:
			return "public_safety_bulletin"
		}
	case "legislative":
		switch {
		case containsAny(lower, "declare war", "declaration of war", "attacked", "invasion", "state of emergency", "martial law"):
			return "strategic_escalation"
		case containsAny(lower, "sanction", "embargo", "designation"):
			return "sanctions_policy"
		default:
			return "official_statement"
		}
	case "informational":
		if containsAny(lower, "sanction", "legislation", "bill", "decree", "executive order", "parliament", "committee") {
			return "policy_update"
		}
		return "context_update"
	default:
		return ""
	}
}

func shouldDowngradeLegislativeToInformational(alert model.Alert, feed FeedContext) bool {
	if !strings.EqualFold(strings.TrimSpace(alert.Category), "legislative") {
		return false
	}
	lowerTitle := strings.ToLower(alert.Title)
	if hasStrategicEscalationTitle(lowerTitle) {
		return false
	}
	if IsActionableTitle(alert.Title) {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		alert.Title,
		feed.Summary,
		feed.Author,
		strings.Join(feed.Tags, " "),
	}, "\n")))
	return hasAny(text, legislativeInformationalTitlePatterns) || IsInformationalTitle(alert.Title)
}

func thresholdForAlert(cfg config.Config, alert model.Alert) float64 {
	if strings.EqualFold(alert.Category, "missing_person") {
		return clamp01(cfg.MissingPersonRelevanceThreshold)
	}
	return clamp01(cfg.IncidentRelevanceThreshold)
}

func defaultSeverity(category string) string {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "informational":
		return "info"
	case "cyber_advisory":
		return "high"
	case "wanted_suspect", "missing_person":
		return "critical"
	case "public_appeal", "humanitarian_tasking", "humanitarian_security", "private_sector":
		return "high"
	case "travel_warning":
		return "high"
	case "maritime_security", "logistics_incident":
		return "high"
	case "environmental_disaster", "disease_outbreak":
		return "high"
	case "emergency_management", "health_emergency":
		return "high"
	default:
		return "medium"
	}
}

func inferSeverity(title string, fallback string) string {
	t := strings.ToLower(title)
	// Informational content overrides keyword-based severity — a conference
	// about pandemics or a review of nuclear infrastructure is not an alert.
	if IsInformationalTitle(title) {
		return "info"
	}
	if fusion := threatFusionMatchCount(t); fusion >= 3 {
		return "critical"
	} else if fusion >= 2 && !strings.EqualFold(fallback, "critical") {
		return "high"
	}
	switch {
	case hasStrategicEscalationTitle(t):
		return "critical"
	case containsAny(t, "critical", "kritische", "emergency", "zero-day", "0-day", "ransomware", "actively exploited", "exploitation", "breach", "data leak", "crypto heist", "million stolen", "wanted", "fugitive", "murder", "homicide", "missing", "amber alert", "kidnap", "do not travel", "notfall", "pandemic", "ebola", "plague", "tsunami", "earthquake", "eruption", "nuclear incident", "radiation leak", "oil spill", "explosion"):
		return "critical"
	case containsAny(t, "hack", "compromise", "vulnerability", "schwachstelle", "sicherheitslücke", "high", "severe", "urgent", "dringend", "fatal", "death", "shooting", "fraud", "scam", "phishing", "reconsider travel", "avoid non-essential travel", "warnung", "gefährlich", "outbreak", "epidemic", "cholera", "mpox", "avian influenza", "flood", "wildfire", "cyclone", "hurricane", "typhoon", "drought", "chemical spill", "hazmat"):
		return "high"
	case containsAny(t, "arrested", "charged", "sentenced", "medium", "moderate", "festgenommen", "verurteilt"):
		return "medium"
	case containsAny(t, "low", "informational", "infopaket", "infoblatt", "handreichung", "leitfaden", "newsletter"):
		return "info"
	default:
		return fallback
	}
}

func parseDate(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	layouts := []string{time.RFC3339, time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC850, "2006-01-02"}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func isFresh(cfg config.Config, date time.Time, now time.Time) bool {
	cutoff := now.Add(-time.Duration(cfg.MaxAgeDays) * 24 * time.Hour)
	return !date.Before(cutoff)
}

func hasAny(text string, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func inferPublicationType(alert model.Alert, feedType string) string {
	if isNewsMedia(alert) {
		return "news_media"
	}
	switch strings.ToLower(alert.Source.AuthorityType) {
	case "cert":
		return "cert_advisory"
	case "police":
		return "law_enforcement"
	case "intelligence", "national_security":
		return "security_bulletin"
	case "public_safety_program":
		return "public_safety_bulletin"
	}
	if feedType == "kev-json" || feedType == "interpol-red-json" || feedType == "interpol-yellow-json" {
		return "structured_incident_feed"
	}
	if feedType == "travelwarning-json" || feedType == "travelwarning-atom" {
		return "official_update"
	}
	return "official_update"
}

func isNewsMedia(alert model.Alert) bool {
	if _, ok := newsMediaIDs[strings.ToLower(alert.SourceID)]; ok {
		return true
	}
	host := extractDomain(alert.CanonicalURL)
	for _, domain := range newsMediaDomains {
		if strings.Contains(host, domain) {
			return true
		}
	}
	return false
}

func isBlog(alert model.Alert) bool {
	if _, ok := blogFilterExempt[strings.ToLower(alert.SourceID)]; ok {
		return false
	}
	title := strings.ToLower(alert.Title)
	link := strings.ToLower(alert.CanonicalURL)
	return strings.Contains(title, "blog") || strings.Contains(link, "/blog") || strings.Contains(link, "medium.com") || strings.Contains(link, "wordpress.com")
}

func isSecurityInformational(alert model.Alert, feed FeedContext) bool {
	text := strings.ToLower(strings.Join([]string{
		alert.Title,
		feed.Summary,
		feed.Author,
		strings.Join(feed.Tags, " "),
		alert.CanonicalURL,
	}, "\n"))
	publicationType := inferPublicationType(alert, feed.FeedType)
	authorityType := strings.ToLower(alert.Source.AuthorityType)
	sourceIsSecurityRelevant := alert.Category == "cyber_advisory" ||
		alert.Category == "private_sector" ||
		publicationType == "cert_advisory" ||
		authorityType == "cert" ||
		authorityType == "private_sector" ||
		authorityType == "regulatory"
	// If the text contains technical indicators (CVE, vulnerability, patch)
	// or actionable directives (update, warning, advisory) it's a real
	// security advisory, not an informational update.
	return sourceIsSecurityRelevant &&
		hasAny(text, securityContextPatterns) &&
		!hasAny(text, incidentDisclosurePatterns) &&
		!hasAny(text, technicalSignalPatterns) &&
		!hasAny(text, actionablePatterns) &&
		!hasAny(text, assistancePatterns) &&
		!hasAny(text, impactSpecificityPatterns) &&
		(hasAny(text, generalNewsPatterns) || hasAny(text, narrativePatterns) || publicationType == "news_media")
}

// IsActionableTitle returns true if the alert title contains words that
// indicate intelligence, security, or safety significance. Used by the
// collection loop to downgrade non-actionable content from broad sources
// (those without include_keywords) to informational severity.
func IsActionableTitle(title string) bool {
	lower := strings.ToLower(title)
	return hasAny(lower, actionableTitlePatterns) ||
		hasAny(lower, technicalSignalPatterns) ||
		hasAny(lower, incidentDisclosurePatterns)
}

// IsInformationalTitle returns true if the title is clearly institutional,
// promotional, or educational content that should not be treated as an alert.
func IsInformationalTitle(title string) bool {
	lower := strings.ToLower(title)
	return hasAny(lower, informationalTitlePatterns)
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func hashID(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}

func jitter(lat float64, lng float64, seed string, geoSource string) (float64, float64) {
	return jitterCC(lat, lng, seed, geoSource, "")
}

func jitterCC(lat float64, lng float64, seed string, geoSource string, countryCode string) (float64, float64) {
	sum := sha1.Sum([]byte(seed))
	angle := float64(sum[0])/255*math.Pi*2 + float64(sum[1])/255
	minRadius, maxRadius := jitterRadiusKM(geoSource)
	// For country-level pins in small countries, clamp jitter so pins
	// stay within the country. For islands, also override center to the
	// interior so coastal capitals don't scatter pins into the sea.
	if sc, ok := smallCountryJitter[countryCode]; ok && isCountryLevel(geoSource) {
		if sc.island {
			lat, lng = sc.lat, sc.lng
		}
		if maxRadius > sc.maxKM {
			maxRadius = sc.maxKM
			minRadius = maxRadius * 0.3
		}
	}
	radius := minRadius + float64(sum[2])/255*(maxRadius-minRadius)
	dLat := (radius / 111.32) * math.Cos(angle)
	cosLat := math.Max(0.2, math.Cos((lat*math.Pi)/180))
	dLng := (radius / (111.32 * cosLat)) * math.Sin(angle)
	outLat := math.Max(-89.5, math.Min(89.5, lat+dLat))
	outLng := lng + dLng
	if outLng > 180 {
		outLng -= 360
	}
	if outLng < -180 {
		outLng += 360
	}
	return round5(outLat), round5(outLng)
}

func jitterRadiusKM(geoSource string) (float64, float64) {
	switch geoSource {
	case "coordinates", "georss":
		return 0.2, 0.8
	case "maritime-region":
		return 5, 25
	case "city-db":
		return 0.4, 1.6
	case "nominatim":
		return 0.8, 2.5
	case "llm":
		return 1, 5
	case "capital", "country-text":
		return 8, 25
	case "registry":
		return 3, 15
	default:
		return 0, 0
	}
}

// isCountryLevel returns true for geo sources where we only know the country,
// not the specific city/location (so we should use the landmass center).
func isCountryLevel(geoSource string) bool {
	return geoSource == "capital" || geoSource == "country-text" || geoSource == "registry"
}

type countryCenter struct {
	lat, lng float64
	maxKM    float64 // max safe jitter radius
	island   bool    // true = override center to landmass interior (coastal capital risk)
}

// smallCountryJitter clamps jitter for small countries.
//
// island=true: override center to the interior of the landmass so pins don't
// scatter into the sea from a coastal capital. Only used for true island
// nations and peninsulas where the capital is on the coast.
//
// island=false: keep the capital/registry coords (where institutions are)
// but clamp the jitter radius so pins stay within the country.
var smallCountryJitter = map[string]countryCenter{
	// Islands / coastal-capital states: override center to interior.
	"MC": {43.74, 7.42, 0.3, true},  // Monaco — city-state
	"SG": {1.35, 103.82, 3, true},   // Singapore
	"BH": {26.07, 50.55, 5, true},   // Bahrain
	"MT": {35.89, 14.44, 2, true},   // Malta — center of main island
	"CY": {35.10, 33.40, 10, true},  // Cyprus
	"JM": {18.15, -77.30, 10, true}, // Jamaica
	"IE": {53.40, -7.69, 25, true},  // Ireland
	"QA": {25.35, 51.18, 10, true},  // Qatar — peninsula
	// Continental: keep capital, just clamp radius.
	"PS": {0, 0, 8, false},
	"LU": {0, 0, 10, false},
	"LB": {0, 0, 12, false},
	"KW": {0, 0, 12, false},
	"IL": {0, 0, 15, false},
	"SI": {0, 0, 15, false},
	"XK": {0, 0, 15, false},
	"ME": {0, 0, 15, false},
	"MK": {0, 0, 15, false},
	"AL": {0, 0, 15, false},
	"BE": {0, 0, 20, false},
	"NL": {0, 0, 20, false},
	"CH": {0, 0, 20, false},
	"DK": {0, 0, 20, false},
	"EE": {0, 0, 20, false},
	"LV": {0, 0, 20, false},
	"LT": {0, 0, 20, false},
	"BA": {0, 0, 20, false},
	"HR": {0, 0, 20, false},
	"SK": {0, 0, 20, false},
	"HU": {0, 0, 25, false},
	"AT": {0, 0, 25, false},
	"CZ": {0, 0, 25, false},
	"RS": {0, 0, 25, false},
}

func extractDomain(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func hoursBetween(now time.Time, publishedAt time.Time) int {
	if publishedAt.IsZero() {
		return 1
	}
	hours := int(math.Round(now.Sub(publishedAt).Hours()))
	if hours < 1 {
		return 1
	}
	return hours
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func round3(value float64) float64 {
	return math.Round(value*1000) / 1000
}

func round5(value float64) float64 {
	return math.Round(value*100000) / 100000
}

func formatDelta(value float64) string {
	return strconvf(value, 2)
}

func strconvf(value float64, places int) string {
	format := math.Pow(10, float64(places))
	value = math.Round(value*format) / format
	return strings.TrimRight(strings.TrimRight(fmtFloat(value), "0"), ".")
}

func fmtFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func limitStrings(values []string, limit int) []string {
	out := make([]string, 0, limit)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
		if len(out) == limit {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

// ApplySignalLanes computes signal lanes (alarm/intel/info) for alert routing.
func ApplySignalLanes(cfg config.Config, alerts []model.Alert) []model.Alert {
	for i := range alerts {
		alerts[i].SignalLane = classifySignalLane(cfg, alerts[i])
	}
	return alerts
}

func classifySignalLane(cfg config.Config, alert model.Alert) model.SignalLane {
	if strings.EqualFold(alert.Category, "informational") || strings.EqualFold(alert.Severity, "info") {
		return model.SignalLaneInfo
	}
	score := 0.0
	if alert.Triage != nil {
		score = alert.Triage.RelevanceScore
	}
	alarmThreshold := clamp01(cfg.AlarmRelevanceThreshold)
	if alarmThreshold == 0 {
		alarmThreshold = 0.72
	}
	if score >= alarmThreshold || strings.EqualFold(alert.Severity, "critical") || strings.EqualFold(alert.Severity, "high") {
		if shouldEscalateThreatFusion(alert, score, alarmThreshold) {
			return model.SignalLaneAlarm
		}
		if hasStrategicEscalationTitle(strings.ToLower(alert.Title)) {
			return model.SignalLaneAlarm
		}
		switch strings.ToLower(strings.TrimSpace(alert.Category)) {
		case "missing_person", "wanted_suspect", "conflict_monitoring", "maritime_security",
			"logistics_incident", "travel_warning", "health_emergency", "disease_outbreak",
			"environmental_disaster", "emergency_management", "terrorism_tip", "public_safety":
			return model.SignalLaneAlarm
		}
		if strings.EqualFold(alert.Source.AuthorityType, "police") || strings.EqualFold(alert.Source.AuthorityType, "national_security") {
			return model.SignalLaneAlarm
		}
	}
	return model.SignalLaneIntel
}

func shouldEscalateThreatFusion(alert model.Alert, score float64, alarmThreshold float64) bool {
	if threatFusionMatchCount(strings.ToLower(alert.Title)) < 2 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(alert.Category)) {
	case "fraud_alert", "public_safety", "terrorism_tip", "conflict_monitoring", "travel_warning":
	default:
		return false
	}
	required := alarmThreshold - 0.08
	if required < 0.6 {
		required = 0.6
	}
	return score >= required || strings.EqualFold(alert.Severity, "critical")
}

func threatFusionMatchCount(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	buckets := []bool{
		containsAny(text, "fraud", "scam", "ponzi", "wire fraud", "investment fraud", "romance scam"),
		containsAny(text, "money laundering", "launder", "sanctions evasion", "illicit finance"),
		containsAny(text, "organized crime", "organised crime", "cartel", "mafia", "racketeer", "transnational criminal", "criminal network"),
		containsAny(text, "terrorism", "terrorist", "extremist", "isis", "isil", "daesh", "al-qaeda", "terror plot"),
	}
	count := 0
	for _, hit := range buckets {
		if hit {
			count++
		}
	}
	return count
}

func hasStrategicEscalationTitle(lowerTitle string) bool {
	return containsAny(lowerTitle,
		"declares war",
		"declaration of war",
		"state of war",
		"country under attack",
		"attacked by",
		"armed attack",
		"missile strike on",
		"invoked article 5",
		"martial law declared",
		"state of emergency declared",
	)
}

func eventGeoConfidence(source string) float64 {
	switch source {
	case "coordinates", "georss", "city-db":
		return 0.95
	case "nominatim":
		return 0.85
	case "llm":
		return 0.65
	case "capital", "country-text":
		return 0.55
	case "registry":
		return 0.35
	default:
		return 0
	}
}

func Deduplicate(alerts []model.Alert) ([]model.Alert, model.DuplicateAudit) {
	// Primary dedup: same canonical URL + title → keep highest-scoring.
	byKey := make(map[string]model.Alert, len(alerts))
	for _, alert := range alerts {
		key := strings.ToLower(alert.CanonicalURL + "|" + alert.Title)
		current, ok := byKey[key]
		if !ok || alertScore(alert) > alertScore(current) {
			byKey[key] = alert
		}
	}
	// Secondary dedup: same AlertID (covers pagination overlap within a
	// source, where URL/title are identical but arrive in separate batches).
	byID := make(map[string]model.Alert, len(byKey))
	for _, alert := range byKey {
		current, ok := byID[alert.AlertID]
		if !ok || alertScore(alert) > alertScore(current) {
			byID[alert.AlertID] = alert
		}
	}
	deduped := make([]model.Alert, 0, len(byID))
	for _, alert := range byID {
		deduped = append(deduped, alert)
	}
	sort.Slice(deduped, func(i, j int) bool { return deduped[i].Title < deduped[j].Title })
	kept, suppressed := collapseVariants(deduped)
	duplicates := summarizeTitleDuplicates(kept)
	return kept, model.DuplicateAudit{
		SuppressedVariantDuplicates: len(suppressed),
		RepeatedTitleGroupsInActive: len(duplicates),
		RepeatedTitleSamples:        duplicates,
	}
}

func FilterActive(cfg config.Config, alerts []model.Alert) (active []model.Alert, filtered []model.Alert) {
	for _, alert := range alerts {
		threshold := thresholdForAlert(cfg, alert)
		score := 0.0
		if alert.Triage != nil {
			score = alert.Triage.RelevanceScore
		}
		if score >= threshold {
			active = append(active, alert)
			continue
		}
		filtered = append(filtered, alert)
	}
	sortAlerts(active, true)
	sortAlerts(filtered, false)
	return active, filtered
}

func sortAlerts(alerts []model.Alert, active bool) {
	sort.Slice(alerts, func(i, j int) bool {
		if !active {
			scoreDelta := alertScore(alerts[j]) - alertScore(alerts[i])
			if scoreDelta != 0 {
				return scoreDelta > 0
			}
		}
		return alerts[i].FirstSeen > alerts[j].FirstSeen
	})
}

func alertScore(alert model.Alert) float64 {
	if alert.Triage == nil {
		return -1
	}
	return alert.Triage.RelevanceScore
}

func collapseVariants(alerts []model.Alert) ([]model.Alert, []model.Alert) {
	byVariant := make(map[string][]model.Alert)
	passthrough := make([]model.Alert, 0, len(alerts))
	for _, alert := range alerts {
		key := buildVariantKey(alert)
		if key == "" {
			passthrough = append(passthrough, alert)
			continue
		}
		byVariant[key] = append(byVariant[key], alert)
	}
	kept := append([]model.Alert{}, passthrough...)
	suppressed := []model.Alert{}
	for _, group := range byVariant {
		if len(group) == 1 {
			kept = append(kept, group[0])
			continue
		}
		sort.Slice(group, func(i, j int) bool {
			return comparePreference(group[i], group[j]) < 0
		})
		kept = append(kept, group[0])
		suppressed = append(suppressed, group[1:]...)
	}
	return kept, suppressed
}

func buildVariantKey(alert model.Alert) string {
	titleNorm := normalizeHeadline(alert.Title)
	if len(titleNorm) < 24 {
		return ""
	}
	u, err := url.Parse(alert.CanonicalURL)
	if err != nil {
		return ""
	}
	path := strings.TrimRight(u.Path, "/")
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) == 0 {
		return ""
	}
	leaf := segments[len(segments)-1]
	re := regexp.MustCompile(`-\d+$`)
	if !re.MatchString(leaf) {
		return ""
	}
	segments[len(segments)-1] = re.ReplaceAllString(leaf, "")
	return strings.ToLower(alert.SourceID + "|" + strings.TrimPrefix(u.Hostname(), "www.") + "/" + strings.Join(segments, "/") + "|" + titleNorm)
}

func comparePreference(a model.Alert, b model.Alert) int {
	if alertScore(a) != alertScore(b) {
		if alertScore(a) > alertScore(b) {
			return -1
		}
		return 1
	}
	if a.FirstSeen != b.FirstSeen {
		if a.FirstSeen > b.FirstSeen {
			return -1
		}
		return 1
	}
	if len(a.CanonicalURL) < len(b.CanonicalURL) {
		return -1
	}
	if len(a.CanonicalURL) > len(b.CanonicalURL) {
		return 1
	}
	return 0
}

func summarizeTitleDuplicates(alerts []model.Alert) []model.DuplicateSample {
	counts := map[string]int{}
	for _, alert := range alerts {
		key := normalizeHeadline(alert.Title)
		if key == "" {
			continue
		}
		counts[key]++
	}
	out := []model.DuplicateSample{}
	for title, count := range counts {
		if count > 1 {
			out = append(out, model.DuplicateSample{Title: title, Count: count})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	if len(out) > 25 {
		out = out[:25]
	}
	return out
}

func normalizeHeadline(value string) string {
	value = strings.ToLower(value)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	return strings.TrimSpace(re.ReplaceAllString(value, " "))
}
