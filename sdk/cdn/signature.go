package cdn

type Signature struct {
	Worker       *SignatureWorker
	Service      *SignatureService
	JobName      string
	JobID        int64
	ProjectKey   string
	WorkflowName string
	WorkflowID   int64
	RunID        int64
	NodeRunName  string
	NodeRunID    int64
	Timestamp    int64
}

type SignatureWorker struct {
	WorkerID     string
	WorkerName   string
	StepOrder    int64
	StepName     string
	ArtifactName string
	FilePerm     uint32
}

type SignatureService struct {
	HatcheryID      int64
	HatcheryName    string
	RequirementID   int64
	RequirementName string
	WorkerName      string
}
