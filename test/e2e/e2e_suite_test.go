package e2e_test

import (
	"testing"
	"time"

	logs "github.com/appscode/go/log/golog"
	crdutils "github.com/appscode/kutil/apiextensions/v1beta1"
	"github.com/appscode/kutil/meta"
	"github.com/appscode/kutil/tools/clientcmd"
	api "github.com/appscode/stash/apis/stash/v1alpha1"
	"github.com/appscode/stash/client/clientset/versioned/scheme"
	_ "github.com/appscode/stash/client/clientset/versioned/scheme"
	"github.com/appscode/stash/pkg/controller"
	"github.com/appscode/stash/pkg/util"
	"github.com/appscode/stash/test/e2e/framework"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	crd_api "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	ka "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
)

const (
	TIMEOUT           = 20 * time.Minute
	TestStashImageTag = "canary"
)

var (
	ctrl *controller.StashController
	root *framework.Framework
)

func TestE2e(t *testing.T) {
	logs.InitLogs()
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(TIMEOUT)
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "e2e Suite", []Reporter{junitReporter})
}

var _ = BeforeSuite(func() {
	scheme.AddToScheme(clientsetscheme.Scheme)
	scheme.AddToScheme(legacyscheme.Scheme)
	util.LoggerOptions.Verbosity = "5"

	clientConfig, err := clientcmd.BuildConfigFromContext(options.KubeConfig, options.KubeContext)
	Expect(err).NotTo(HaveOccurred())

	ctrlConfig := controller.NewConfig(clientConfig)

	err = options.ApplyTo(ctrlConfig)
	Expect(err).NotTo(HaveOccurred())

	kaClient := ka.NewForConfigOrDie(clientConfig)

	root = framework.New(ctrlConfig.KubeClient, ctrlConfig.StashClient, kaClient, options.EnableWebhook, options.SelfHostedOperator, clientConfig)
	err = root.CreateTestNamespace()
	Expect(err).NotTo(HaveOccurred())
	By("Using test namespace " + root.Namespace())

	crds := []*crd_api.CustomResourceDefinition{
		api.Restic{}.CustomResourceDefinition(),
		api.Recovery{}.CustomResourceDefinition(),
		api.Repository{}.CustomResourceDefinition(),
	}

	By("Registering CRDs")
	err = crdutils.RegisterCRDs(ctrlConfig.CRDClient, crds)
	Expect(err).NotTo(HaveOccurred())

	if !options.SelfHostedOperator {
		go root.StartAPIServerAndOperator(options.KubeConfig, options.ExtraOptions)
		root.EventuallyAPIServerReady().Should(Succeed())
		// let's API server be warmed up
		time.Sleep(time.Second * 5)
	}
})

var _ = AfterSuite(func() {
	By("Cleaning API server and Webhook stuff")

	if options.EnableWebhook && !options.SelfHostedOperator {
		root.KubeClient.AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Delete("admission.stash.appscode.com", meta.DeleteInBackground())
		root.KubeClient.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Delete("admission.stash.appscode.com", meta.DeleteInBackground())
	}

	if !options.SelfHostedOperator {
		root.KubeClient.CoreV1().Endpoints(root.Namespace()).Delete("stash-local-apiserver", meta.DeleteInBackground())
		root.KubeClient.CoreV1().Services(root.Namespace()).Delete("stash-local-apiserver", meta.DeleteInBackground())
		root.KAClient.ApiregistrationV1beta1().APIServices().Delete("v1alpha1.admission.stash.appscode.com", meta.DeleteInBackground())
	}
	root.DeleteNamespace(root.Namespace())
})
