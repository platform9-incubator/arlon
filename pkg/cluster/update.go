package cluster

import (
	"context"
	"fmt"
	argoapp "github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	argoappv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	arlonv1 "github.com/arlonproj/arlon/api/v1"
	"github.com/arlonproj/arlon/pkg/clusterspec"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

func Update(
	appIf argoapp.ApplicationServiceClient,
	config *restclient.Config,
	argocdNs,
	arlonNs,
	clusterName,
	clusterSpecName string,
	prof *arlonv1.Profile,
	updateInArgoCd bool,
	managementClusterUrl string,
	oldApp *argoappv1.Application,
) (*argoappv1.Application, error) {
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get kube client: %s", err)
	}
	corev1 := kubeClient.CoreV1()
	configMapsApi := corev1.ConfigMaps(arlonNs)
	clusterSpecCm, err := configMapsApi.Get(context.Background(), clusterSpecName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get clusterspec configmap: %s", err)
	}
	// Ensure subchart name (api, cloud, clustertype) hasn't changed
	subchartName, err := clusterspec.SubchartName(clusterSpecCm)
	helmParamName := fmt.Sprintf("tags.%s", subchartName)
	found := false
	for _, param := range oldApp.Spec.Source.Helm.Parameters {
		found = param.Name == helmParamName && param.Value == "true"
		if found {
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("the api provider, cloud provider, or cluster type cannot change")
	}
	repoUrl := oldApp.Spec.Source.RepoURL
	repoBranch := oldApp.Spec.Source.TargetRevision
	repoPath := oldApp.Spec.Source.Path
	basePath, clstName, err := decomposePath(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to decompose repo path: %s", err)
	}
	if clstName != clusterName {
		return nil, fmt.Errorf("unexpected cluster name extracted from repo path: %s",
			clstName)
	}
	rootApp, err := ConstructRootApp(argocdNs, clusterName, repoUrl,
		repoBranch, repoPath, clusterSpecName, clusterSpecCm, prof.Name,
		managementClusterUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to construct root app: %s", err)
	}
	if oldApp.Spec.Source.RepoURL != rootApp.Spec.Source.RepoURL ||
		oldApp.Spec.Source.Path != rootApp.Spec.Source.Path {
		return nil, fmt.Errorf("git repo reference cannot change")
	}
	err = DeployToGit(config, argocdNs, arlonNs, clusterName,
		repoUrl, repoBranch, basePath, prof)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy git tree: %s", err)
	}
	if updateInArgoCd {
		appUpdateRequest := argoapp.ApplicationUpdateRequest{
			Application: rootApp,
		}
		_, err := appIf.Update(context.Background(), &appUpdateRequest)
		if err != nil {
			return nil, fmt.Errorf("failed to update ArgoCD root application: %s", err)
		}
	}
	return rootApp, nil
}
