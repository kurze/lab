package main

type CommitReviewStore interface {
	IsCommitReviewed(project, sha string) bool
	MarkCommitReviewed(project, sha string)
}

type Storage interface {
	IsReviewed(project string, id int64) bool
	MarkReviewed(project string, id int64)
	CommitReviewStore
	StoreResult(project string, sr *StoredResult)
	GetResult(project, key string) *StoredResult
	ListResults(project string) []*StoredResult
	Save() error
}

func LoadStorage(path string) (Storage, error) {
	return LoadState(path)
}
