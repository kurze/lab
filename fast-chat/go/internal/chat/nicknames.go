package chat

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// Docker-style adjectives
var adjectives = []string{
	"admiring", "adoring", "affectionate", "agitated", "amazing",
	"angry", "awesome", "beautiful", "blissful", "bold",
	"boring", "brave", "busy", "charming", "clever",
	"cool", "compassionate", "competent", "confident", "cranky",
	"crazy", "dazzling", "determined", "distracted", "dreamy",
	"eager", "ecstatic", "elastic", "elated", "elegant",
	"eloquent", "epic", "exciting", "fervent", "festive",
	"flamboyant", "focused", "friendly", "frosty", "funny",
	"gallant", "gifted", "goofy", "gracious", "happy",
	"hardcore", "heuristic", "hopeful", "hungry", "infallible",
	"inspiring", "interesting", "intelligent", "jolly", "jovial",
	"keen", "kind", "laughing", "loving", "lucid",
	"magical", "mystifying", "modest", "musing", "naughty",
	"nervous", "nice", "nifty", "nostalgic", "objective",
	"optimistic", "peaceful", "pedantic", "pensive", "practical",
	"priceless", "quirky", "quizzical", "recursing", "relaxed",
	"reverent", "romantic", "sad", "serene", "sharp",
	"silly", "sleepy", "stoic", "strange", "stupefied",
	"suspicious", "sweet", "tender", "thirsty", "trusting",
	"unruffled", "upbeat", "vibrant", "vigilant", "vigorous",
	"wizardly", "wonderful", "xenodochial", "youthful", "zealous",
}

// Famous scientists and engineers
var names = []string{
	"albattani", "allen", "almeida", "antonelli", "agnesi",
	"archimedes", "ardinghelli", "aryabhata", "austin", "babbage",
	"banach", "banzai", "bardeen", "bartik", "bassi",
	"beaver", "bell", "benz", "bhabha", "bhaskara",
	"black", "blackburn", "blackwell", "bohr", "booth",
	"borg", "bose", "bouman", "boyd", "brahmagupta",
	"brattain", "brown", "buck", "burnell", "cannon",
	"carson", "cartwright", "carver", "cerf", "chandrasekhar",
	"chaplygin", "chatelet", "chatterjee", "chebyshev", "cohen",
	"chaum", "clarke", "colden", "cori", "cray",
	"curie", "curran", "darwin", "davinci", "dewdney",
	"dhawan", "diffie", "dijkstra", "dirac", "driscoll",
	"dubinsky", "easley", "edison", "einstein", "elbakyan",
	"elgamal", "elion", "ellis", "engelbart", "euclid",
	"euler", "faraday", "feistel", "fermat", "fermi",
	"feynman", "franklin", "gagarin", "galileo", "galois",
	"ganguly", "gates", "gauss", "germain", "goldberg",
	"goldstine", "goldwasser", "golick", "goodall", "gould",
	"greider", "grothendieck", "haibt", "hamilton", "haslett",
	"hawking", "hellman", "heisenberg", "hermann", "herschel",
	"hertz", "heyrovsky", "hodgkin", "hofstadter", "hoover",
	"hopper", "hugle", "hypatia", "ishizaka", "jackson",
	"jang", "jemison", "jennings", "jepsen", "johnson",
	"joliot", "jones", "kalam", "kapitsa", "kare",
	"keldysh", "keller", "kepler", "khayyam", "khorana",
	"kilby", "kirch", "knuth", "kowalevski", "lalande",
	"lamarr", "lamport", "leakey", "leavitt", "lederberg",
	"lehmann", "lewin", "lichterman", "liskov", "lovelace",
	"lumiere", "mahavira", "margulis", "matsumoto", "maxwell",
	"mayer", "mccarthy", "mcclintock", "mclaren", "mclean",
	"mcnulty", "mendel", "mendeleev", "meitner", "meninsky",
	"merkle", "mestorf", "mirzakhani", "moore", "morse",
	"murdock", "moser", "napier", "nash", "neumann",
	"newton", "nightingale", "nobel", "noether", "northcutt",
	"noyce", "panini", "pare", "pascal", "pasteur",
	"payne", "perlman", "pike", "poincare", "poitras",
	"proskuriakova", "ptolemy", "raman", "ramanujan", "ride",
	"montalcini", "ritchie", "rhodes", "robinson", "roentgen",
	"rosalind", "rubin", "saha", "sammet", "sanderson",
	"satoshi", "shamir", "shannon", "shaw", "shirley",
	"shockley", "shtern", "sinoussi", "snyder", "solomon",
	"spence", "stonebraker", "sutherland", "swanson", "swartz",
	"swirles", "taussig", "tereshkova", "tesla", "tharp",
	"thompson", "torvalds", "tu", "turing", "varahamihira",
	"vaughan", "visvesvaraya", "volhard", "villani", "wescoff",
	"wilbur", "wiles", "williams", "williamson", "wilson",
	"wing", "wozniak", "wright", "wu", "yalow",
	"yonath", "zhukovsky",
}

const (
	initialPoolSize = 100 // Initial batch of pre-generated nicknames
	refillThreshold = 20  // Trigger refill when pool drops below this
	refillBatchSize = 50  // Generate this many nicknames per refill
)

// NicknamePool manages a pool of available nicknames
type NicknamePool struct {
	available     []string
	used          map[string]bool
	mu            sync.Mutex
	rng           *rand.Rand
	availableSize atomic.Int32 // Cache of len(available) for lock-free reads
	usedSize      atomic.Int32 // Cache of len(used) for lock-free reads
}

// NewNicknamePool creates a new nickname pool
func NewNicknamePool() *NicknamePool {
	pool := &NicknamePool{
		available: make([]string, 0, initialPoolSize),
		used:      make(map[string]bool),
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	// Generate initial small batch
	pool.generateNicknames(initialPoolSize)
	pool.availableSize.Store(int32(len(pool.available)))
	pool.usedSize.Store(0)

	return pool
}

// generateNicknames generates n unique nicknames not in the blacklist (used map)
// NOTE: Must be called while holding p.mu lock
func (p *NicknamePool) generateNicknames(n int) {
	generated := 0
	attempts := 0
	maxAttempts := n * 10 // Prevent infinite loops if space is exhausted

	for generated < n && attempts < maxAttempts {
		adj := adjectives[p.rng.Intn(len(adjectives))]
		name := names[p.rng.Intn(len(names))]
		nickname := adj + "_" + name

		// Check blacklist (used map) and avoid duplicates in available pool
		if !p.used[nickname] && !p.contains(nickname) {
			p.available = append(p.available, nickname)
			generated++
		}
		attempts++
	}
}

// contains checks if nickname is already in available pool
func (p *NicknamePool) contains(nickname string) bool {
	for _, n := range p.available {
		if n == nickname {
			return true
		}
	}
	return false
}

// Allocate gets the next available nickname
func (p *NicknamePool) Allocate() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Auto-refill if pool is running low
	if len(p.available) < refillThreshold {
		p.generateNicknames(refillBatchSize)
	}

	// If still empty (very rare, all combinations exhausted)
	if len(p.available) == 0 {
		// Emergency fallback: generate until we find one not in blacklist
		for {
			adj := adjectives[p.rng.Intn(len(adjectives))]
			name := names[p.rng.Intn(len(names))]
			nickname := adj + "_" + name
			if !p.used[nickname] {
				p.used[nickname] = true
				p.usedSize.Add(1)
				return nickname
			}
		}
	}

	// Pop from the end for O(1) performance
	nickname := p.available[len(p.available)-1]
	p.available = p.available[:len(p.available)-1]
	p.used[nickname] = true

	// Update atomic counters
	p.availableSize.Add(-1)
	p.usedSize.Add(1)

	return nickname
}

// Release returns a nickname to the pool
func (p *NicknamePool) Release(nickname string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Only release if it was actually allocated
	if !p.used[nickname] {
		return
	}

	delete(p.used, nickname)
	p.available = append(p.available, nickname)

	// Update atomic counters
	p.availableSize.Add(1)
	p.usedSize.Add(-1)
}

// Available returns the number of available nicknames (lock-free read)
func (p *NicknamePool) Available() int {
	return int(p.availableSize.Load())
}

// Used returns the number of used nicknames (lock-free read)
func (p *NicknamePool) Used() int {
	return int(p.usedSize.Load())
}
