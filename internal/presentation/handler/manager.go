package handler

import ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"

func GetAccessList(manager ports.Manager) ([]ports.AccessInfo, error) {
	return manager.GetAccessList()
}

func AddAccess(manager ports.Manager, name string, number string, expiredAt int64) error {
	return manager.AddAccess(name, number, expiredAt)
}

func DeleteAccess(manager ports.Manager, number string) error {
	return manager.DeleteAccess(number)
}
