package engine

// OCCAxisNames is the full 22-emotion axis set of the OCC model
// (Ortony, Clore & Collins 1988), grouped by appraisal branch.
var OCCAxisNames = []string{
	// well-being (consequences of events, for self, actual)
	"joy", "distress",
	// prospect-based (for self, uncertain, then resolved)
	"hope", "fear", "satisfaction", "disappointment", "relief", "fears-confirmed",
	// fortunes-of-others (consequences for others x liking)
	"happy-for", "pity", "resentment", "gloating",
	// attribution (actions of agents x praiseworthiness)
	"pride", "shame", "admiration", "reproach",
	// attraction (aspects of objects)
	"love", "hate",
	// compounds (well-being x attribution)
	"gratification", "gratitude", "remorse", "anger",
}
