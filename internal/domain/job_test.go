package domain

import "testing"

func TestCreateJobRequestValidate(t *testing.T) {
	valid := CreateJobRequest{
		SourceType: "s3_presigned",
		Pipeline: []PipelineStep{
			{
				ID:     "thumb_small",
				Action: "resize",
			},
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid request, got error: %v", err)
	}

	invalid := CreateJobRequest{}
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected validation error for empty request")
	}
}
