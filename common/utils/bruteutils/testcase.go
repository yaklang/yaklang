package bruteutils

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/mixer"
)

func runTest(r *DefaultServiceAuthInfo, target ...string) error {
	swg := utils.NewSizedWaitGroup(10)
	defer swg.Wait()
	err := mixer.MixForEach([][]string{
		r.DefaultUsernames,
		r.DefaultPasswords,
	}, func(i ...string) error {
		for _, t := range target {
			t := t
			swg.Add()
			go func() {
				defer swg.Done()

				result := r.BrutePass(&BruteItem{
					Type:     r.ServiceName,
					Target:   t,
					Username: i[0],
					Password: i[1],
				})
				result.Show()
			}()
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
