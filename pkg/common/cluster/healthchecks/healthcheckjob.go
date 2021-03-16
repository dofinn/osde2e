package healthchecks

import (
	"context"
	"fmt"
	"log"

	"github.com/openshift/osde2e/pkg/common/logging"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

// CheckHealthcheckJob uses the `osd-cluster-ready` healthcheck job to determine cluster readiness.
func CheckHealthcheckJob(k8sClient *kubernetes.Clientset, ctx context.Context, logger *log.Logger) (bool, error) {
	logger = logging.CreateNewStdLoggerOrUseExistingLogger(logger)

	logger.Print("Checking whether cluster is healthy before proceeding...")

	bv1C := k8sClient.BatchV1()
	namespace := "openshift-monitoring"
	name := "osd-cluster-ready"
	jobs, err := bv1C.Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed listing jobs: %w", err)
	}
	for _, job := range jobs.Items {
		if job.Name != name {
			continue
		}
		if job.Status.Succeeded > 0 {
			log.Println("Healthcheck job has already succeeded")
			return true, nil
		}
		log.Println("Healthcheck job has not yet succeeded, watching...")
	}
	watcher, err := bv1C.Jobs(namespace).Watch(ctx, metav1.ListOptions{
		ResourceVersion: jobs.ResourceVersion,
		FieldSelector:   "metadata.name=osd-cluster-ready",
	})
	if err != nil {
		return false, fmt.Errorf("failed watching job: %w", err)
	}
	for {
		select {
		case event := <-watcher.ResultChan():
			switch event.Type {
			case watch.Added:
				fallthrough
			case watch.Modified:
				job := event.Object.(*batchv1.Job)
				if job.Status.Succeeded > 0 {
					return true, nil
				}
			case watch.Deleted:
				return false, fmt.Errorf("cluster readiness job deleted before becoming ready")
			case watch.Error:
				return false, fmt.Errorf("watch returned error event: %v", event)
			default:
				logger.Printf("Unrecognized event type while watching for healthcheck job updates: %v", event.Type)
			}
		case <-ctx.Done():
			return false, fmt.Errorf("healtcheck watch context cancelled while still waiting for success")
		}
	}
}
