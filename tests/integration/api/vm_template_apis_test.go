package api_test

import (
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/config"
	ctlapisv1alpha1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	. "github.com/rancher/harvester/tests/framework/dsl"
	"github.com/rancher/harvester/tests/framework/fuzz"
	"github.com/rancher/harvester/tests/framework/helper"
)

const (
	defaultVMTemplates        = 3
	defaultVMTemplateVersions = 3
)

var _ = Describe("verify vm template APIs", func() {

	var (
		scaled            *config.Scaled
		templates         ctlapisv1alpha1.VirtualMachineTemplateClient
		templateVersions  ctlapisv1alpha1.VirtualMachineTemplateVersionClient
		templateNamespace string
	)

	BeforeEach(func() {
		scaled = harvester.Scaled()
		templates = scaled.HarvesterFactory.Harvester().V1alpha1().VirtualMachineTemplate()
		templateVersions = scaled.HarvesterFactory.Harvester().V1alpha1().VirtualMachineTemplateVersion()
		templateNamespace = config.Namespace
	})

	Cleanup(func() {
		templateList, err := templates.List(templateNamespace, metav1.ListOptions{
			LabelSelector: labels.FormatLabels(testResourceLabels)})
		if err != nil {
			GinkgoT().Logf("failed to list tested vm templates, %v", err)
			return
		}
		for _, item := range templateList.Items {
			if err = templates.Delete(item.Namespace, item.Name, &metav1.DeleteOptions{}); err != nil {
				GinkgoT().Logf("failed to delete tested template %s/%s, %v", item.Namespace, item.Name, err)
			}
		}

		templateVersionList, err := templateVersions.List(templateNamespace, metav1.ListOptions{
			LabelSelector: labels.FormatLabels(testResourceLabels)})
		if err != nil {
			GinkgoT().Logf("failed to list tested vm templates, %v", err)
			return
		}
		for _, item := range templateVersionList.Items {
			if err = templateVersions.Delete(item.Namespace, item.Name, &metav1.DeleteOptions{}); err != nil {
				GinkgoT().Logf("failed to delete tested template version %s/%s, %v", item.Namespace, item.Name, err)
			}
		}
	})

	Context("operate via steve API", func() {

		var (
			template = v1alpha1.VirtualMachineTemplate{
				ObjectMeta: v1.ObjectMeta{
					Name:      "vm-template-0",
					Namespace: templateNamespace,
					Labels:    testResourceLabels,
				},
				Spec: v1alpha1.VirtualMachineTemplateSpec{
					Description: "testing vm template",
				},
			}
			templateVersion = v1alpha1.VirtualMachineTemplateVersion{
				ObjectMeta: v1.ObjectMeta{
					Name:      fuzz.String(5),
					Namespace: templateNamespace,
					Labels:    testResourceLabels,
				},
				Spec: v1alpha1.VirtualMachineTemplateVersionSpec{},
			}
			templateAPI, templateVersionAPI string
		)

		BeforeEach(func() {

			templateAPI = helper.BuildAPIURL("v1", "harvester.cattle.io.virtualmachinetemplates")
			templateVersionAPI = helper.BuildAPIURL("v1", "harvester.cattle.io.virtualmachinetemplateversions")

		})

		Specify("verify default vm templates", func() {

			By("list default templates", func() {

				templates, respCode, respBody, err := helper.GetCollection(templateAPI)
				MustRespCodeIs(http.StatusOK, "get templates", err, respCode, respBody)
				MustEqual(len(templates.Data), defaultVMTemplates)

			})

			By("list default template versions", func() {

				templates, respCode, respBody, err := helper.GetCollection(templateVersionAPI)
				MustRespCodeIs(http.StatusOK, "get template versions", err, respCode, respBody)
				MustEqual(len(templates.Data), defaultVMTemplateVersions)

			})

		})

		Specify("verify the vm template and template versions", func() {

			var (
				templateID         = fmt.Sprintf("%s/%s", templateNamespace, template.Name)
				defaultVersionID   = fmt.Sprintf("%s/%s", templateNamespace, templateVersion.Name)
				templateURL        = helper.BuildResourceURL(templateAPI, templateNamespace, template.Name)
				templateVersionURL = helper.BuildResourceURL(templateVersionAPI, templateNamespace, templateVersion.Name)
			)

			By("create a vm template", func() {

				respCode, respBody, err := helper.PostObjectByYAML(templateAPI, template)
				MustRespCodeIs(http.StatusCreated, "create template", err, respCode, respBody)

			})

			By("create a vm template version without templateID", func() {

				respCode, respBody, err := helper.PostObjectByYAML(templateVersionAPI, templateVersion)
				MustRespCodeIs(http.StatusUnprocessableEntity, "create template version", err, respCode, respBody)

			})

			By("create a vm template version", func() {

				templateVersion.Spec.TemplateID = templateID
				templateVersion.Spec.VM = NewDefaultTestVMBuilder().vm.Spec

				respCode, respBody, err := helper.PostObjectByYAML(templateVersionAPI, templateVersion)
				MustRespCodeIs(http.StatusCreated, "create template version", err, respCode, respBody)

			})

			By("then validate vm template version", func() {
				MustFinallyBeTrue(func() bool {
					respCode, respBody, err := helper.GetObject(templateURL, &template)
					MustRespCodeIs(http.StatusOK, "get vm template", err, respCode, respBody)
					if template.Spec.DefaultVersionID == defaultVersionID &&
						template.Status.DefaultVersion == 1 && template.Status.LatestVersion == 1 {
						return true
					}
					return false
				}, 3)
			})

			By("can't delete a default template version", func() {

				respCode, respBody, err := helper.DeleteObject(templateVersionURL)
				MustRespCodeIs(http.StatusInternalServerError, "delete template version", err, respCode, respBody)

			})

			By("delete a template", func() {

				respCode, respBody, err := helper.DeleteObject(templateURL)
				MustRespCodeIn("delete template", err, respCode, respBody, http.StatusOK, http.StatusNoContent)

			})

			By("then validate total vm templates", func() {

				MustFinallyBeTrue(func() bool {
					templates, respCode, respBody, err := helper.GetCollection(templateAPI)
					MustRespCodeIs(http.StatusOK, "get template", err, respCode, respBody)
					return len(templates.Data) == defaultVMTemplates
				}, 3)

			})

			By("then validate total vm template versions", func() {

				MustFinallyBeTrue(func() bool {
					templates, respCode, respBody, err := helper.GetCollection(templateVersionAPI)
					MustRespCodeIs(http.StatusOK, "get template versions", err, respCode, respBody)
					return len(templates.Data) == defaultVMTemplateVersions
				}, 3)

			})

		})

	})

})
