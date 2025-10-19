use rand::Rng;
use std::collections::HashSet;
use std::sync::atomic::{AtomicI32, Ordering};

const ADJECTIVES: &[&str] = &[
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
];

const NAMES: &[&str] = &[
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
];

const INITIAL_POOL_SIZE: usize = 100;
const REFILL_THRESHOLD: usize = 20;
const REFILL_BATCH_SIZE: usize = 50;

pub struct NicknamePool {
    available: Vec<String>,
    used: HashSet<String>,
    shuffled_pool: Vec<String>,
    shuffle_index: usize,
    available_size: AtomicI32,
    used_size: AtomicI32,
}

impl NicknamePool {
    pub fn new() -> Self {
        let mut pool = Self {
            available: Vec::with_capacity(INITIAL_POOL_SIZE),
            used: HashSet::new(),
            shuffled_pool: Vec::new(),
            shuffle_index: 0,
            available_size: AtomicI32::new(0),
            used_size: AtomicI32::new(0),
        };

        pool.generate_nicknames(INITIAL_POOL_SIZE);
        pool.available_size.store(pool.available.len() as i32, Ordering::Relaxed);
        pool
    }

    fn generate_nicknames(&mut self, n: usize) {
        let mut rng = rand::thread_rng();
        let mut generated = 0;
        let mut attempts = 0;
        let max_attempts = n * 10;

        while generated < n && attempts < max_attempts {
            let adj = ADJECTIVES[rng.gen_range(0..ADJECTIVES.len())];
            let name = NAMES[rng.gen_range(0..NAMES.len())];
            let nickname = format!("{}_{}", adj, name);

            if !self.used.contains(&nickname) && !self.contains(&nickname) {
                self.available.push(nickname);
                generated += 1;
            }
            attempts += 1;
        }
    }

    fn contains(&self, nickname: &str) -> bool {
        self.available.iter().any(|n| n == nickname)
    }

    pub fn allocate(&mut self) -> String {
        let total_combinations = ADJECTIVES.len() * NAMES.len();
        let used_count = self.used.len();
        let collision_probability = used_count as f64 / total_combinations as f64;

        if collision_probability > 0.5 && self.shuffled_pool.is_empty() {
            self.initialize_shuffled_pool();
        }

        if !self.shuffled_pool.is_empty() {
            return self.allocate_from_shuffled_pool();
        }

        if self.available.len() < REFILL_THRESHOLD {
            self.generate_nicknames(REFILL_BATCH_SIZE);
        }

        if self.available.is_empty() {
            loop {
                let mut rng = rand::thread_rng();
                let adj = ADJECTIVES[rng.gen_range(0..ADJECTIVES.len())];
                let name = NAMES[rng.gen_range(0..NAMES.len())];
                let nickname = format!("{}_{}", adj, name);
                if !self.used.contains(&nickname) {
                    self.used.insert(nickname.clone());
                    self.used_size.fetch_add(1, Ordering::Relaxed);
                    return nickname;
                }
            }
        }

        let nickname = self.available.pop().unwrap();
        self.used.insert(nickname.clone());

        self.available_size.fetch_sub(1, Ordering::Relaxed);
        self.used_size.fetch_add(1, Ordering::Relaxed);

        nickname
    }

    fn initialize_shuffled_pool(&mut self) {
        self.shuffled_pool.clear();
        
        for adj in ADJECTIVES {
            for name in NAMES {
                let nickname = format!("{}_{}", adj, name);
                if !self.used.contains(&nickname) {
                    self.shuffled_pool.push(nickname);
                }
            }
        }

        let mut rng = rand::thread_rng();
        for i in (1..self.shuffled_pool.len()).rev() {
            let j = rng.gen_range(0..=i);
            self.shuffled_pool.swap(i, j);
        }
        self.shuffle_index = 0;
    }

    fn allocate_from_shuffled_pool(&mut self) -> String {
        if self.shuffle_index >= self.shuffled_pool.len() {
            self.shuffled_pool.clear();
            self.shuffle_index = 0;
            return self.allocate();
        }

        let nickname = self.shuffled_pool[self.shuffle_index].clone();
        self.shuffle_index += 1;
        self.used.insert(nickname.clone());
        self.used_size.fetch_add(1, Ordering::Relaxed);

        nickname
    }

    pub fn release(&mut self, nickname: String) {
        if !self.used.remove(&nickname) {
            return;
        }

        self.available.push(nickname);
        self.available_size.fetch_add(1, Ordering::Relaxed);
        self.used_size.fetch_sub(1, Ordering::Relaxed);
    }

    #[allow(dead_code)]
    pub fn available(&self) -> usize {
        self.available_size.load(Ordering::Relaxed) as usize
    }

    #[allow(dead_code)]
    pub fn used(&self) -> usize {
        self.used_size.load(Ordering::Relaxed) as usize
    }
}

impl Default for NicknamePool {
    fn default() -> Self {
        Self::new()
    }
}
