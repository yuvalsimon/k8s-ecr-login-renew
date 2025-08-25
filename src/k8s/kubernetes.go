package k8s

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os/user"
	"path/filepath"
	"strings"

	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type config struct {
	Auths map[string]*auth `json:"auths"`
}

type auth struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Auth     string `json:"auth"`
}

const defaultEmail = "awsregrenew@demo.test"

func GetClient() (*kubernetes.Clientset, error) {
	config, err := getClientConfig()
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

func getClientConfig() (*rest.Config, error) {
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	u, err := user.Current()
	if nil != err {
		return nil, err
	}

	return clientcmd.BuildConfigFromFlags("", filepath.Join(u.HomeDir, ".kube", "config"))
}

func getSecret(client *kubernetes.Clientset, name, namespace string) (*coreV1.Secret, error) {
	secret, err := client.CoreV1().Secrets(namespace).Get(context.TODO(), name, metaV1.GetOptions{})
	if nil != err {
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		return nil, err
	}

	return secret, nil
}

func getConfig(username, password string, servers []string) ([]byte, error) {
	config := config{Auths: make(map[string]*auth, len(servers))}

	for _, server := range servers {
		config.Auths[server] = &auth{
			Username: username,
			Password: password,
			Email:    defaultEmail,
			Auth:     base64.StdEncoding.EncodeToString([]byte(username + ":" + password)),
		}
	}

	configJson, err := json.Marshal(config)
	if nil != err {
		return nil, err
	}
	return configJson, nil
}

func createSecret(name string, annotations map[string]string) *coreV1.Secret {
	secret := coreV1.Secret{}
	secret.Name = name
	secret.Type = coreV1.SecretTypeDockerConfigJson
	secret.Data = map[string][]byte{}
	if len(annotations) > 0 {
		secret.Annotations = annotations
	}
	return &secret
}

func UpdatePassword(namespace, name, username, password string, servers []string, annotations map[string]string) error {
	client, err := GetClient()
	if nil != err {
		return err
	}

	secret, err := getSecret(client, name, namespace)
	if nil != err {
		return err
	}

	configJson, err := getConfig(username, password, servers)
	if nil != err {
		return err
	}

	if secret == nil {
		secret = createSecret(name, annotations)
		secret.Data[coreV1.DockerConfigJsonKey] = configJson
		_, err = client.CoreV1().Secrets(namespace).Create(context.TODO(), secret, metaV1.CreateOptions{})
		return err
	}

	secret.Data[coreV1.DockerConfigJsonKey] = configJson
	// Apply annotations to existing secret
	if len(annotations) > 0 {
		if secret.Annotations == nil {
			secret.Annotations = make(map[string]string)
		}
		for key, value := range annotations {
			secret.Annotations[key] = value
		}
	}
	_, err = client.CoreV1().Secrets(namespace).Update(context.TODO(), secret, metaV1.UpdateOptions{})

	if err == nil {
		return nil
	}

	// fall back to delete+create in case permissions are not updated
	err = client.CoreV1().Secrets(namespace).Delete(context.TODO(), name, metaV1.DeleteOptions{})
	if err != nil {
		return err
	}

	secret = createSecret(name, annotations)
	secret.Data[coreV1.DockerConfigJsonKey] = configJson
	_, err = client.CoreV1().Secrets(namespace).Create(context.TODO(), secret, metaV1.CreateOptions{})
	return err
}
