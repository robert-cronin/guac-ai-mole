package tools

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Khan/genqlient/graphql"
	model "github.com/guacsec/guac/pkg/assembler/clients/generated"
	"github.com/guacsec/guac/pkg/assembler/helpers"
)

// known.go is a close replica of guacone's known query functionality but redesigned as a tool for LLMs

type knownQueryInput struct {
	SubjectType string `json:"subjectType"`
	Subject     string `json:"subject"`
}

type knownResult struct {
	Sections      []knownSection `json:"sections"`
	VisualizerUrl string         `json:"visualizerUrl,omitempty"`
}

// knownSection represents a table-like section of results
type knownSection struct {
	Title string     `json:"title"`
	Rows  []knownRow `json:"rows"`
	Edges []string   `json:"edges,omitempty"` // node IDs to be appended to path
}

type knownRow struct {
	NodeType  string `json:"nodeType"`
	NodeID    string `json:"nodeId"`
	ExtraInfo string `json:"extraInfo"`
}

const (
	hashEqualStr        = "hashEqual"
	scorecardStr        = "scorecard"
	occurrenceStr       = "occurrence"
	hasSrcAtStr         = "hasSrcAt"
	hasSBOMStr          = "hasSBOM"
	hasSLSAStr          = "hasSLSA"
	certifyVulnStr      = "certifyVuln"
	certifyLegalStr     = "certifyLegal"
	vexLinkStr          = "vexLink"
	badLinkStr          = "badLink"
	goodLinkStr         = "goodLink"
	pkgEqualStr         = "pkgEqual"
	packageSubjectType  = "package"
	sourceSubjectType   = "source"
	artifactSubjectType = "artifact"
	noVulnType          = "noVuln"
)

func KnownQuery(ctx context.Context, client graphql.Client, input knownQueryInput) (interface{}, error) {
	switch input.SubjectType {
	case packageSubjectType:
		return knownQueryPackage(ctx, client, input.Subject)
	case sourceSubjectType:
		return knownQuerySource(ctx, client, input.Subject)
	case artifactSubjectType:
		return knownQueryArtifact(ctx, client, input.Subject)
	default:
		return nil, fmt.Errorf("invalid subjectType: must be package, source, or artifact")
	}
}

// replicate logic from guacone query known for package
func knownQueryPackage(ctx context.Context, client graphql.Client, purl string) (interface{}, error) {
	pkgInput, err := helpers.PurlToPkg(purl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PURL: %w", err)
	}

	pkgQualifierFilter := []model.PackageQualifierSpec{}
	for _, qualifier := range pkgInput.Qualifiers {
		qualifier := qualifier
		pkgQualifierFilter = append(pkgQualifierFilter, model.PackageQualifierSpec{
			Key:   qualifier.Key,
			Value: &qualifier.Value,
		})
	}

	pkgFilter := &model.PkgSpec{
		Type:       &pkgInput.Type,
		Namespace:  pkgInput.Namespace,
		Name:       &pkgInput.Name,
		Version:    pkgInput.Version,
		Subpath:    pkgInput.Subpath,
		Qualifiers: pkgQualifierFilter,
	}
	pkgResponse, err := model.Packages(ctx, client, *pkgFilter)
	if err != nil {
		return nil, err
	}
	if len(pkgResponse.Packages) != 1 {
		return nil, fmt.Errorf("failed to locate package based on purl")
	}

	// Package Name Node
	pkgNameNodeID := pkgResponse.Packages[0].Namespaces[0].Names[0].Id
	nameNeighbors, namePath, err := queryKnownNeighbors(ctx, client, pkgNameNodeID)
	if err != nil {
		return nil, fmt.Errorf("error querying for package name neighbors: %v", err)
	}

	packageNameNodesSection := knownSection{
		Title: "Package Name Nodes",
		Rows:  []knownRow{},
		Edges: namePath,
	}
	// hasSrcAt, badLink, goodLink for package name nodes
	packageNameNodesSection.Rows = append(packageNameNodesSection.Rows, getOutputBasedOnNode(ctx, client, nameNeighbors, hasSrcAtStr, packageSubjectType)...)
	packageNameNodesSection.Rows = append(packageNameNodesSection.Rows, getOutputBasedOnNode(ctx, client, nameNeighbors, badLinkStr, packageSubjectType)...)
	packageNameNodesSection.Rows = append(packageNameNodesSection.Rows, getOutputBasedOnNode(ctx, client, nameNeighbors, goodLinkStr, packageSubjectType)...)

	// Build name node path
	namePathFull := []string{
		pkgResponse.Packages[0].Namespaces[0].Names[0].Id,
		pkgResponse.Packages[0].Namespaces[0].Id,
		pkgResponse.Packages[0].Id,
	}
	namePathFull = append(namePathFull, namePath...)

	// Package Version Node
	pkgVersionNodeID := pkgResponse.Packages[0].Namespaces[0].Names[0].Versions[0].Id
	versionNeighbors, versionPath, err := queryKnownNeighbors(ctx, client, pkgVersionNodeID)
	if err != nil {
		return nil, fmt.Errorf("error querying for package version neighbors: %v", err)
	}

	packageVersionNodesSection := knownSection{
		Title: "Package Version Nodes",
		Rows:  []knownRow{},
		Edges: versionPath,
	}
	// add rows for version nodes
	packageVersionNodesSection.Rows = append(packageVersionNodesSection.Rows, getOutputBasedOnNode(ctx, client, versionNeighbors, certifyVulnStr, packageSubjectType)...)
	packageVersionNodesSection.Rows = append(packageVersionNodesSection.Rows, getOutputBasedOnNode(ctx, client, versionNeighbors, hasSBOMStr, packageSubjectType)...)
	packageVersionNodesSection.Rows = append(packageVersionNodesSection.Rows, getOutputBasedOnNode(ctx, client, versionNeighbors, occurrenceStr, packageSubjectType)...)
	packageVersionNodesSection.Rows = append(packageVersionNodesSection.Rows, getOutputBasedOnNode(ctx, client, versionNeighbors, certifyLegalStr, packageSubjectType)...)
	packageVersionNodesSection.Rows = append(packageVersionNodesSection.Rows, getOutputBasedOnNode(ctx, client, versionNeighbors, hasSLSAStr, packageSubjectType)...)
	packageVersionNodesSection.Rows = append(packageVersionNodesSection.Rows, getOutputBasedOnNode(ctx, client, versionNeighbors, vexLinkStr, packageSubjectType)...)
	packageVersionNodesSection.Rows = append(packageVersionNodesSection.Rows, getOutputBasedOnNode(ctx, client, versionNeighbors, pkgEqualStr, packageSubjectType)...)
	packageVersionNodesSection.Rows = append(packageVersionNodesSection.Rows, getOutputBasedOnNode(ctx, client, versionNeighbors, badLinkStr, packageSubjectType)...)
	packageVersionNodesSection.Rows = append(packageVersionNodesSection.Rows, getOutputBasedOnNode(ctx, client, versionNeighbors, goodLinkStr, packageSubjectType)...)

	versionPathFull := []string{
		pkgResponse.Packages[0].Namespaces[0].Names[0].Versions[0].Id,
		pkgResponse.Packages[0].Namespaces[0].Names[0].Id,
		pkgResponse.Packages[0].Namespaces[0].Id,
		pkgResponse.Packages[0].Id,
	}
	versionPathFull = append(versionPathFull, versionPath...)

	result := knownResult{
		Sections: []knownSection{
			packageNameNodesSection,
			packageVersionNodesSection,
		},
		VisualizerUrl: fmt.Sprintf("http://localhost:3000/?path=%v", strings.Join(removeDuplicateValuesFromPath(append(namePathFull, versionPathFull...)), ",")),
	}

	return result, nil
}

func knownQuerySource(ctx context.Context, client graphql.Client, subject string) (interface{}, error) {
	srcInput, err := helpers.VcsToSrc(subject)
	if err != nil {
		return nil, fmt.Errorf("failed to parse source: %v", err)
	}

	srcFilter := &model.SourceSpec{
		Type:      &srcInput.Type,
		Namespace: &srcInput.Namespace,
		Name:      &srcInput.Name,
		Tag:       srcInput.Tag,
		Commit:    srcInput.Commit,
	}
	srcResponse, err := model.Sources(ctx, client, *srcFilter)
	if err != nil {
		return nil, err
	}
	if len(srcResponse.Sources) != 1 {
		return nil, fmt.Errorf("failed to locate source based on input")
	}

	sourceNodeID := srcResponse.Sources[0].Namespaces[0].Names[0].Id
	sourceNeighbors, sourcePath, err := queryKnownNeighbors(ctx, client, sourceNodeID)
	if err != nil {
		return nil, fmt.Errorf("error querying for source neighbors: %v", err)
	}

	// Create a single section for source since we don't have multiple sections like package
	sourceSection := knownSection{
		Title: "Source Nodes",
		Rows:  []knownRow{},
		Edges: sourcePath,
	}
	sourceSection.Rows = append(sourceSection.Rows, getOutputBasedOnNode(ctx, client, sourceNeighbors, hasSrcAtStr, sourceSubjectType)...)
	sourceSection.Rows = append(sourceSection.Rows, getOutputBasedOnNode(ctx, client, sourceNeighbors, occurrenceStr, sourceSubjectType)...)
	sourceSection.Rows = append(sourceSection.Rows, getOutputBasedOnNode(ctx, client, sourceNeighbors, scorecardStr, sourceSubjectType)...)
	sourceSection.Rows = append(sourceSection.Rows, getOutputBasedOnNode(ctx, client, sourceNeighbors, badLinkStr, sourceSubjectType)...)
	sourceSection.Rows = append(sourceSection.Rows, getOutputBasedOnNode(ctx, client, sourceNeighbors, goodLinkStr, sourceSubjectType)...)

	fullPath := []string{
		srcResponse.Sources[0].Namespaces[0].Names[0].Id,
		srcResponse.Sources[0].Namespaces[0].Id,
		srcResponse.Sources[0].Id,
	}
	fullPath = append(fullPath, sourcePath...)

	result := knownResult{
		Sections: []knownSection{sourceSection},
		VisualizerUrl: fmt.Sprintf("http://localhost:3000/?path=%v",
			strings.Join(removeDuplicateValuesFromPath(fullPath), ",")),
	}

	return result, nil
}

func knownQueryArtifact(ctx context.Context, client graphql.Client, subject string) (interface{}, error) {
	split := strings.Split(subject, ":")
	if len(split) != 2 {
		return nil, fmt.Errorf("artifact must be in algorithm:digest form")
	}
	algorithm := strings.ToLower(split[0])
	digest := strings.ToLower(split[1])

	artifactFilter := &model.ArtifactSpec{
		Algorithm: &algorithm,
		Digest:    &digest,
	}

	artifactResponse, err := model.Artifacts(ctx, client, *artifactFilter)
	if err != nil {
		return nil, err
	}
	if len(artifactResponse.Artifacts) != 1 {
		return nil, fmt.Errorf("failed to locate artifact based on (algorithm:digest)")
	}

	artifactNodeID := artifactResponse.Artifacts[0].Id
	artifactNeighbors, artifactPath, err := queryKnownNeighbors(ctx, client, artifactNodeID)
	if err != nil {
		return nil, fmt.Errorf("error querying for artifact neighbors: %v", err)
	}

	artifactSection := knownSection{
		Title: "Artifact Nodes",
		Rows:  []knownRow{},
		Edges: artifactPath,
	}
	artifactSection.Rows = append(artifactSection.Rows, getOutputBasedOnNode(ctx, client, artifactNeighbors, hashEqualStr, artifactSubjectType)...)
	artifactSection.Rows = append(artifactSection.Rows, getOutputBasedOnNode(ctx, client, artifactNeighbors, occurrenceStr, artifactSubjectType)...)
	artifactSection.Rows = append(artifactSection.Rows, getOutputBasedOnNode(ctx, client, artifactNeighbors, hasSBOMStr, artifactSubjectType)...)
	artifactSection.Rows = append(artifactSection.Rows, getOutputBasedOnNode(ctx, client, artifactNeighbors, hasSLSAStr, artifactSubjectType)...)
	artifactSection.Rows = append(artifactSection.Rows, getOutputBasedOnNode(ctx, client, artifactNeighbors, vexLinkStr, artifactSubjectType)...)
	artifactSection.Rows = append(artifactSection.Rows, getOutputBasedOnNode(ctx, client, artifactNeighbors, badLinkStr, artifactSubjectType)...)
	artifactSection.Rows = append(artifactSection.Rows, getOutputBasedOnNode(ctx, client, artifactNeighbors, goodLinkStr, artifactSubjectType)...)

	fullPath := []string{artifactResponse.Artifacts[0].Id}
	fullPath = append(fullPath, artifactPath...)

	result := knownResult{
		Sections: []knownSection{artifactSection},
		VisualizerUrl: fmt.Sprintf("http://localhost:3000/?path=%v",
			strings.Join(removeDuplicateValuesFromPath(fullPath), ",")),
	}

	return result, nil
}

// below helper functions adapt from the cli code but store results in JSON structures

type neighbors struct {
	hashEquals    []*model.NeighborsNeighborsHashEqual
	scorecards    []*model.NeighborsNeighborsCertifyScorecard
	occurrences   []*model.NeighborsNeighborsIsOccurrence
	hasSrcAt      []*model.NeighborsNeighborsHasSourceAt
	hasSBOMs      []*model.NeighborsNeighborsHasSBOM
	hasSLSAs      []*model.NeighborsNeighborsHasSLSA
	certifyVulns  []*model.NeighborsNeighborsCertifyVuln
	certifyLegals []*model.NeighborsNeighborsCertifyLegal
	vexLinks      []*model.NeighborsNeighborsCertifyVEXStatement
	badLinks      []*model.NeighborsNeighborsCertifyBad
	goodLinks     []*model.NeighborsNeighborsCertifyGood
	pkgEquals     []*model.NeighborsNeighborsPkgEqual
}

func queryKnownNeighbors(ctx context.Context, gqlclient graphql.Client, subjectQueryID string) (*neighbors, []string, error) {
	collectedNeighbors := &neighbors{}
	var path []string
	neighborResponse, err := model.Neighbors(ctx, gqlclient, subjectQueryID, []model.Edge{})
	if err != nil {
		return nil, nil, fmt.Errorf("error querying neighbors: %v", err)
	}
	for _, neighbor := range neighborResponse.Neighbors {
		switch v := neighbor.(type) {
		case *model.NeighborsNeighborsCertifyVuln:
			collectedNeighbors.certifyVulns = append(collectedNeighbors.certifyVulns, v)
			path = append(path, v.Id)
		case *model.NeighborsNeighborsCertifyBad:
			collectedNeighbors.badLinks = append(collectedNeighbors.badLinks, v)
			path = append(path, v.Id)
		case *model.NeighborsNeighborsCertifyGood:
			collectedNeighbors.goodLinks = append(collectedNeighbors.goodLinks, v)
			path = append(path, v.Id)
		case *model.NeighborsNeighborsCertifyScorecard:
			collectedNeighbors.scorecards = append(collectedNeighbors.scorecards, v)
			path = append(path, v.Id)
		case *model.NeighborsNeighborsCertifyVEXStatement:
			collectedNeighbors.vexLinks = append(collectedNeighbors.vexLinks, v)
			path = append(path, v.Id)
		case *model.NeighborsNeighborsHasSBOM:
			collectedNeighbors.hasSBOMs = append(collectedNeighbors.hasSBOMs, v)
			path = append(path, v.Id)
		case *model.NeighborsNeighborsHasSLSA:
			collectedNeighbors.hasSLSAs = append(collectedNeighbors.hasSLSAs, v)
			path = append(path, v.Id)
		case *model.NeighborsNeighborsHasSourceAt:
			collectedNeighbors.hasSrcAt = append(collectedNeighbors.hasSrcAt, v)
			path = append(path, v.Id)
		case *model.NeighborsNeighborsHashEqual:
			collectedNeighbors.hashEquals = append(collectedNeighbors.hashEquals, v)
			path = append(path, v.Id)
		case *model.NeighborsNeighborsIsOccurrence:
			collectedNeighbors.occurrences = append(collectedNeighbors.occurrences, v)
			path = append(path, v.Id)
		case *model.NeighborsNeighborsPkgEqual:
			collectedNeighbors.pkgEquals = append(collectedNeighbors.pkgEquals, v)
			path = append(path, v.Id)
		case *model.NeighborsNeighborsCertifyLegal:
			collectedNeighbors.certifyLegals = append(collectedNeighbors.certifyLegals, v)
			path = append(path, v.Id)
		default:
			continue
		}
	}
	return collectedNeighbors, path, nil
}

func getOutputBasedOnNode(ctx context.Context, gqlclient graphql.Client, collectedNeighbors *neighbors, nodeType string, subjectType string) []knownRow {
	logger := slog.With("function", "getOutputBasedOnNode")
	var rows []knownRow
	switch nodeType {
	case certifyVulnStr:
		for _, certVuln := range collectedNeighbors.certifyVulns {
			if certVuln.Vulnerability.Type != noVulnType {
				for _, vuln := range certVuln.Vulnerability.VulnerabilityIDs {
					rows = append(rows, knownRow{certifyVulnStr, certVuln.Id, "vulnerability ID: " + vuln.VulnerabilityID})
				}
			} else {
				rows = append(rows, knownRow{certifyVulnStr, certVuln.Id, "vulnerability ID: " + noVulnType})
			}
		}
	case badLinkStr:
		for _, bad := range collectedNeighbors.badLinks {
			rows = append(rows, knownRow{badLinkStr, bad.Id, "justification: " + bad.Justification})
		}
	case goodLinkStr:
		for _, good := range collectedNeighbors.goodLinks {
			rows = append(rows, knownRow{goodLinkStr, good.Id, "justification: " + good.Justification})
		}
	case scorecardStr:
		for _, score := range collectedNeighbors.scorecards {
			rows = append(rows, knownRow{scorecardStr, score.Id, "Overall Score: " + fmt.Sprintf("%f", score.Scorecard.AggregateScore)})
		}
	case vexLinkStr:
		for _, vex := range collectedNeighbors.vexLinks {
			rows = append(rows, knownRow{vexLinkStr, vex.Id, fmt.Sprintf("Vex Status: %s", vex.Status)})
		}
	case hasSBOMStr:
		if len(collectedNeighbors.hasSBOMs) > 0 {
			for _, sbom := range collectedNeighbors.hasSBOMs {
				rows = append(rows, knownRow{hasSBOMStr, sbom.Id, "SBOM Download Location: " + sbom.DownloadLocation})
			}
		} else {
			for _, occurrence := range collectedNeighbors.occurrences {
				neighborResponseHasSBOM, err := getAssociatedArtifact(ctx, gqlclient, occurrence, model.EdgeArtifactHasSbom)
				if err != nil {
					logger.Debug("error querying neighbors: %v", err)
				} else {
					for _, neighborHasSBOM := range neighborResponseHasSBOM.Neighbors {
						if hasSBOM, ok := neighborHasSBOM.(*model.NeighborsNeighborsHasSBOM); ok {
							rows = append(rows, knownRow{hasSBOMStr, hasSBOM.Id, "SBOM Download Location: " + hasSBOM.DownloadLocation})
						}
					}
				}
			}
		}
	case hasSLSAStr:
		if len(collectedNeighbors.hasSLSAs) > 0 {
			for _, slsa := range collectedNeighbors.hasSLSAs {
				rows = append(rows, knownRow{hasSLSAStr, slsa.Id, "SLSA Attestation Location: " + slsa.Slsa.Origin})
			}
		} else {
			for _, occurrence := range collectedNeighbors.occurrences {
				neighborResponseHasSLSA, err := getAssociatedArtifact(ctx, gqlclient, occurrence, model.EdgeArtifactHasSlsa)
				if err != nil {
					logger.Debug("error querying neighbors: %v", err)
				} else {
					for _, neighborHasSLSA := range neighborResponseHasSLSA.Neighbors {
						if hasSLSA, ok := neighborHasSLSA.(*model.NeighborsNeighborsHasSLSA); ok {
							rows = append(rows, knownRow{hasSLSAStr, hasSLSA.Id, "SLSA Attestation Location: " + hasSLSA.Slsa.Origin})
						}
					}
				}
			}
		}
	case hasSrcAtStr:
		for _, src := range collectedNeighbors.hasSrcAt {
			if subjectType == packageSubjectType {
				namespace := src.Source.Namespaces[0].Namespace
				if !strings.HasPrefix(namespace, "https://") {
					namespace = "https://" + namespace
				}
				rows = append(rows, knownRow{hasSrcAtStr, src.Id, "Source: " + src.Source.Type + "+" + namespace + "/" + src.Source.Namespaces[0].Names[0].Name})
			} else {
				purl := helpers.PkgToPurl(src.Package.Type, src.Package.Namespaces[0].Namespace,
					src.Package.Namespaces[0].Names[0].Name, "", "", []string{})
				rows = append(rows, knownRow{hasSrcAtStr, src.Id, "Source for Package: " + purl})
			}
		}
	case hashEqualStr:
		for _, hash := range collectedNeighbors.hashEquals {
			rows = append(rows, knownRow{hashEqualStr, hash.Id, ""})
		}
	case occurrenceStr:
		for _, occurrence := range collectedNeighbors.occurrences {
			if subjectType == artifactSubjectType {
				switch v := occurrence.Subject.(type) {
				case *model.AllIsOccurrencesTreeSubjectPackage:
					purl := helpers.PkgToPurl(v.Type, v.Namespaces[0].Namespace,
						v.Namespaces[0].Names[0].Name, v.Namespaces[0].Names[0].Versions[0].Version, "", []string{})
					rows = append(rows, knownRow{occurrenceStr, occurrence.Id, "Occurrence for Package: " + purl})
				case *model.AllIsOccurrencesTreeSubjectSource:
					namespace := v.Namespaces[0].Namespace
					if !strings.HasPrefix(namespace, "https://") {
						namespace = "https://" + namespace
					}
					rows = append(rows, knownRow{occurrenceStr, occurrence.Id, "Occurrence for Source: " + v.Type + "+" + namespace + "/" + v.Namespaces[0].Names[0].Name})
				}
			} else {
				rows = append(rows, knownRow{occurrenceStr, occurrence.Id, "Occurrence for Artifact: " + occurrence.Artifact.Algorithm + ":" + occurrence.Artifact.Digest})
			}
		}
	case pkgEqualStr:
		for _, equal := range collectedNeighbors.pkgEquals {
			rows = append(rows, knownRow{pkgEqualStr, equal.Id, ""})
		}
	case certifyLegalStr:
		for _, legal := range collectedNeighbors.certifyLegals {
			rows = append(rows, knownRow{
				certifyLegalStr,
				legal.Id,
				"Declared License: " + legal.DeclaredLicense +
					", Discovered License: " + legal.DiscoveredLicense +
					", Origin: " + legal.Origin,
			})
		}
	}

	return rows
}

func getAssociatedArtifact(ctx context.Context, gqlclient graphql.Client, occurrence *model.NeighborsNeighborsIsOccurrence, edge model.Edge) (*model.NeighborsResponse, error) {
	logger := slog.With("function", "getAssociatedArtifact")
	artifactFilter := &model.ArtifactSpec{
		Algorithm: &occurrence.Artifact.Algorithm,
		Digest:    &occurrence.Artifact.Digest,
	}
	artifactResponse, err := model.Artifacts(ctx, gqlclient, *artifactFilter)
	if err != nil {
		logger.Debug("error querying for artifacts: %v", err)
		return nil, err
	}
	if len(artifactResponse.Artifacts) != 1 {
		logger.Debug("failed to located artifacts based on (algorithm:digest)")
		return nil, fmt.Errorf("artifact not found")
	}
	return model.Neighbors(ctx, gqlclient, artifactResponse.Artifacts[0].Id, []model.Edge{edge})
}

func removeDuplicateValuesFromPath(path []string) []string {
	seen := make(map[string]bool)
	result := []string{}
	for _, p := range path {
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	return result
}
