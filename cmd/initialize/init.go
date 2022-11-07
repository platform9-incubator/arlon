package initialize

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/account"
	"github.com/argoproj/argo-cd/v2/util/cli"
	argocdio "github.com/argoproj/argo-cd/v2/util/io"
	"github.com/argoproj/argo-cd/v2/util/localconfig"
	"github.com/arlonproj/arlon/config"
	"github.com/arlonproj/arlon/deploy"
	"github.com/arlonproj/arlon/pkg/argocd"
	gyaml "github.com/ghodss/yaml"
	"github.com/spf13/cobra"
	"io"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"net/http"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
)

const (
	argocdManifestURL              = "https://raw.githubusercontent.com/argoproj/argo-cd/%s/manifests/install.yaml"
	defaultArgoNamespace           = "argocd"
	defaultArlonNamespace          = "arlon"
	defaultArlonArgoCDUser         = "arlon"
	defaultArgoServerDeployment    = "argocd-server"
	reasonMinimumReplicasAvailable = "MinimumReplicasAvailable"
)

var argocdGitTag string = "release-2.4"

func NewCommand() *cobra.Command {
	var argoCfgPath string
	var cliConfig clientcmd.ClientConfig
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Run the init command",
		Long:  "Run the init command",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cfg, err := cliConfig.ClientConfig()
			if err != nil {
				return err
			}
			client, err := k8sclient.New(cfg, k8sclient.Options{})
			if err != nil {
				return err
			}
			kubeClient := kubernetes.NewForConfigOrDie(cfg)

			//canInstallArgo, err := canInstallArgocd()
			if err != nil {
				return err
			}
			if true {
				fmt.Println("Cannot initialize argocd client. Argocd may not be installed")
				shouldInstallArgo := cli.AskToProceed("argo-cd not found, possibly not installed. Proceed to install? [y/n]")
				if shouldInstallArgo {
					if err := beginArgoCDInstall(ctx, client, kubeClient); err != nil {
						return err
					}
				}
			}
			argoClient := argocd.NewArgocdClientOrDie("")
			//canInstall, err := canInstallArlon(ctx, kubeClient)
			if err != nil {
				return err
			}
			if true {
				fmt.Println("arlon namespace not found. Arlon controller might not be installed")
				//shouldInstallArlon := cli.AskToProceed("Install arlon controller? [y/n]")
				cli.AskToProceed("portforward argo and login using admin credentials")
				if true {
					if err := beginArlonInstall(ctx, client, kubeClient, argoClient, defaultArlonNamespace, defaultArgoNamespace); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
	cliConfig = cli.AddKubectlFlagsToCmd(cmd)
	cmd.Flags().StringVar(&argoCfgPath, "argo-cfg", "", "Path to argocd configuration file")
	return cmd
}

func beginArlonInstall(ctx context.Context, client k8sclient.Client, kubeClient *kubernetes.Clientset, argoClient apiclient.Client, arlonNs, argoNs string) error {
	ns, err := kubeClient.CoreV1().Namespaces().Create(ctx, &v1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      arlonNs,
			Namespace: arlonNs,
		},
	}, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	if errors.IsAlreadyExists(err) {
		fmt.Printf("namespace %s already exists\n", arlonNs)
	} else {
		fmt.Printf("namespage %s created\n", ns.GetName())
	}
	argoCm, err := kubeClient.CoreV1().ConfigMaps(argoNs).Get(ctx, "argocd-cm", metav1.GetOptions{})
	if err != nil {
		return err
	}
	argoCm.Data = map[string]string{
		"accounts.arlon": "apiKey, login",
	}
	argoRbacCm, err := kubeClient.CoreV1().ConfigMaps(argoNs).Get(ctx, "argocd-rbac-cm", metav1.GetOptions{})
	if err != nil {
		return err
	}
	argoRbacCm.Data = map[string]string{
		"policy.csv": "g, arlon, role:admin",
	}
	cm, err := kubeClient.CoreV1().ConfigMaps(argoNs).Update(ctx, argoCm, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	fmt.Printf("ConfigMap %s updated\n", cm.GetName())

	rbacCm, err := kubeClient.CoreV1().ConfigMaps(argoNs).Update(ctx, argoRbacCm, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	fmt.Printf("ConfigMap %s updated\n", rbacCm.GetName())
	sec, err := createArgoCreds(ctx, kubeClient, argoClient, arlonNs, argoNs)
	if err != nil {
		return err
	}
	fmt.Printf("Secret %s created", sec.GetName())
	crds := [][]byte{
		config.CRDProfile,
		config.CRDClusterReg,
		config.CRDCallHomeConfig,
	}
	deplManifests := [][]byte{
		deploy.YAMLdeploy,
		deploy.YAMLrbacCHC,
		deploy.YAMLrbacClusterReg,
		deploy.YAMLwebhook,
	}
	decodedCrds := [][]*unstructured.Unstructured{}
	for _, crd := range crds {
		decoded, err := decodeResources(crd)
		if err != nil {
			return err
		}
		decodedCrds = append(decodedCrds, decoded)
	}
	decodedDeplManifests := [][]*unstructured.Unstructured{}
	for _, manifest := range deplManifests {
		decoded, err := decodeResources(manifest)
		if err != nil {
			return err
		}
		decodedDeplManifests = append(decodedDeplManifests, decoded)
	}

	for i := 0; i < len(decodedCrds); i++ {
		for j := 0; j < len(decodedCrds[i]); j++ {
			if err := applyObject(ctx, client, decodedCrds[i][j], arlonNs); err != nil {
				return err
			}
		}
	}

	for i := 0; i < len(decodedDeplManifests); i++ {
		for j := 0; j < len(decodedDeplManifests[i]); j++ {
			if err := applyObject(ctx, client, decodedDeplManifests[i][j], arlonNs); err != nil {
				return err
			}
		}
	}
	return nil
}

func beginArgoCDInstall(ctx context.Context, client k8sclient.Client, kubeClient *kubernetes.Clientset) error {
	downloadLink := fmt.Sprintf(argocdManifestURL, argocdGitTag)
	err := client.Create(ctx, &v1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind: "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultArgoNamespace,
		},
	})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	if err := installArgo(downloadLink, client); err != nil {
		return err
	}
	err = wait.PollImmediate(time.Second*10, time.Minute*5, func() (bool, error) {
		fmt.Printf("waiting for argocd-server")
		var deployment *apps.Deployment
		d, err := kubeClient.AppsV1().Deployments(defaultArgoNamespace).Get(ctx, defaultArgoServerDeployment, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		deployment = d
		condition := getDeploymentCondition(deployment.Status, apps.DeploymentAvailable)
		return condition != nil && condition.Reason == reasonMinimumReplicasAvailable, nil
	})
	if err != nil {
		return err
	}
	return nil
}

func canInstallArgocd() (bool, error) {
	return true, nil
}

func canInstallArlon(ctx context.Context, kubeClient *kubernetes.Clientset) (bool, error) {
	if _, err := kubeClient.CoreV1().Namespaces().Get(ctx, defaultArlonNamespace, metav1.GetOptions{}); err != nil {
		if errors.IsNotFound(err) {
			return true, nil
		}
	}
	return false, nil
}

func installArgo(downloadLink string, client k8sclient.Client) error {
	manifest, err := downloadManifest(downloadLink)
	if err != nil {
		return err
	}
	resources, err := decodeResources(manifest)
	if err != nil {
		return err
	}
	for _, obj := range resources {
		err := applyObject(context.Background(), client, obj, defaultArgoNamespace)
		if err != nil {
			return err
		}
	}
	return nil
}

func decodeResources(manifest []byte) ([]*unstructured.Unstructured, error) {
	var resources []*unstructured.Unstructured
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(manifest), 4096)
	for {
		resource := unstructured.Unstructured{}
		err := decoder.Decode(&resource)
		if err == nil {
			resources = append(resources, &resource)
		} else if err == io.EOF {
			break
		} else {
			return nil, err
		}
	}
	return resources, nil
}

func applyObject(ctx context.Context, client k8sclient.Client, object *unstructured.Unstructured, namespace string) error {
	name := object.GetName()
	object.SetNamespace(namespace)
	if name == "" {
		return fmt.Errorf("object %s has no name", object.GroupVersionKind().String())
	}
	groupVersionKind := object.GroupVersionKind()
	objDesc := fmt.Sprintf("(%s) %s/%s", groupVersionKind.String(), namespace, name)
	err := client.Create(ctx, object)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			fmt.Printf("%s already exists\n", objDesc)
			return nil
		}
		return fmt.Errorf("could not create %s. Error: %v", objDesc, err.Error())
	}
	fmt.Printf("successfully created %s", objDesc)
	return nil
}

func downloadManifest(link string) ([]byte, error) {
	client := http.Client{
		Timeout: 30 * time.Second,
	}
	res, err := client.Get(link)
	if err != nil {
		return nil, err
	}
	defer argocdio.Close(res.Body)
	respBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return respBytes, nil
}

func getDeploymentCondition(status apps.DeploymentStatus, condType apps.DeploymentConditionType) *apps.DeploymentCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

func createArgoCreds(ctx context.Context, clientset *kubernetes.Clientset, argoClient apiclient.Client, arlonNamespace string, argoNamespace string) (*v1.Secret, error) {
	conn, accountClient := argoClient.NewAccountClientOrDie()
	defer argocdio.Close(conn)
	res, err := accountClient.CreateToken(ctx, &account.CreateTokenRequest{
		Name:      defaultArlonArgoCDUser,
		ExpiresIn: 0,
	})
	if err != nil {
		return nil, err
	}
	defaultInClusterUser := fmt.Sprintf("%s.%s.svc.cluster.local", defaultArgoServerDeployment, argoNamespace)
	argoCfg := localconfig.LocalConfig{
		CurrentContext: defaultInClusterUser,
		Contexts: []localconfig.ContextRef{
			{
				Name:   defaultInClusterUser,
				Server: defaultInClusterUser,
				User:   defaultInClusterUser,
			},
		},
		Servers: []localconfig.Server{
			{
				Server:          defaultInClusterUser,
				Insecure:        true,
				GRPCWebRootPath: "",
			},
		},
		Users: []localconfig.User{
			{
				Name:      defaultInClusterUser,
				AuthToken: res.GetToken(),
			},
		},
	}
	out, err := gyaml.Marshal(argoCfg)
	if err != nil {
		return nil, err
	}
	secret := v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "argocd-creds",
			Namespace: arlonNamespace,
		},
		Data: nil,
		Type: v1.SecretTypeOpaque,
	}
	secret.Data = map[string][]byte{
		"config": out,
	}
	created, err := clientset.CoreV1().Secrets(arlonNamespace).Create(ctx, &secret, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func portForward(ctx context.Context, client kubernetes.Clientset) error {
	// use this command to get the argocd pod ➜  ~ kubectl get pods -l app.kubernetes.io/name=argocd-server -o yaml
	pods, err := client.CoreV1().Pods(defaultArgoNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=argocd-server",
	})
	if err != nil {
		return err
	}
	if len(pods.Items) == 0 {
		return errors.NewNotFound(schema.GroupResource{
			Group:    "v1",
			Resource: "pod",
		}, defaultArgoServerDeployment)
	}
	for _, pod := range pods.Items {
		if strings.Contains(pod.Name, defaultArgoServerDeployment) {
			// run port forward
			runPortForward(ctx, client)
			break
		}
	}
	return nil
}

func runPortForward(ctx context.Context, client kubernetes.Clientset) {

}
