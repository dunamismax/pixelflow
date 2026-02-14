package domain

import "testing"

func TestCreateJobRequestValidate(t *testing.T) {
	valid := CreateJobRequest{
		SourceType: SourceTypeS3Presigned,
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

	missingObjectKey := CreateJobRequest{
		SourceType: SourceTypeLocalFile,
		Pipeline: []PipelineStep{
			{
				ID:     "thumb_small",
				Action: "resize",
			},
		},
	}
	if err := missingObjectKey.Validate(); err == nil {
		t.Fatal("expected validation error for local_file object_key")
	}

	unsupportedSourceType := CreateJobRequest{
		SourceType: "http_url",
		Pipeline: []PipelineStep{
			{
				ID:     "thumb_small",
				Action: "resize",
			},
		},
	}
	if err := unsupportedSourceType.Validate(); err == nil {
		t.Fatal("expected validation error for unsupported source_type")
	}
}
