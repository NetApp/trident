package sync

import (
	"fmt"
	"time"

	v1 "github.com/netapp/trident/persistent_store/kubernetes/apis/netapp/v1"
	clientv1 "github.com/netapp/trident/persistent_store/kubernetes/client/clientset/versioned/typed/netapp/v1"
	"github.com/pborman/uuid"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	// Interval to poll for mutex lock
	pollInterval = time.Second

	// Time to keep the mutex lease
	leaseDuration = time.Second * 10

	// Interval to update the mutex lease
	updateInterval = time.Second * 2
)

// Now function, declared here for easy testing
var Now = func() time.Time {
	return time.Now().UTC()
}

// Mutex is a mutex within Kubernetes using CRD objects
type Mutex struct {
	uuid   string
	client clientv1.MutexInterface
}

// Lock is a locked resource using a Kubernetes Mutex
type Lock struct {
	done chan struct{}
}

// NewMutex creates a manager for named mutex locks using Kubernetes CRDs
func NewMutex(client clientv1.MutexInterface) *Mutex {
	return &Mutex{
		uuid:   uuid.New(),
		client: client,
	}
}

// Unlock releases the locks currently owned
func (l *Lock) Unlock() {
	// Some safety around closing the channel more then once
	select {
	case <-l.done:
		return
	default:
		close(l.done)
	}
}

// Lock creates one or more named locks and returns an object that can be used
// to close the locks obtained
func (m *Mutex) Lock(names ...string) (*Lock, error) {
	group := errgroup.Group{}
	done := make(chan struct{})

	// Try for all locks at once
	for _, name := range names {
		n := name // clone the name

		group.Go(func() error {
			return m.lockOne(n, done)
		})
	}

	// Wait for all locks to be obtained
	err := group.Wait()

	// If any attempts to obtain locks errors close the
	// channel to release any we got
	if err != nil {
		close(done)
		return nil, err
	}

	// Return the lock object
	return &Lock{
		done: done,
	}, nil
}

// lockOne waits forever until a lock is obtained, once it is obtained it is kept
// until the done channel is closed
func (m *Mutex) lockOne(name string, done chan struct{}) error {
	var mutex *v1.Mutex

	// Try for lock forever
	err := wait.PollUntil(pollInterval, func() (done bool, err error) {
		// Remove zombie mutexes
		err = m.removeExpiredMutex(name)
		if err != nil {
			return false, err
		}

		log.Debugf("Trying for lock: %s", name)

		// Try for lock
		mutex, done, err = m.createMutex(name)
		return
	}, done)

	// If done was closed early then return
	if mutex == nil {
		return nil
	}

	// Pass errors up
	if err != nil {
		return err
	}

	log.Debugf("Lock obtained: %s", name)

	// Start goroutine to keep ownership
	go m.keepOwnership(mutex, done)

	// We have the lock
	return nil
}

// createMutex will try and create a mutex object in Kubernetes
// it should not error if a mutex already exists, just return false
func (m *Mutex) createMutex(name string) (mutex *v1.Mutex, locked bool, err error) {
	// Try and create mutex
	mutex, err = m.client.Create(&v1.Mutex{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "Mutex",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Holder:  m.uuid,
		Expires: Now().Add(leaseDuration).Unix(),
	})

	// Only return error if its not a "Allready Exists" error
	if errors.IsAlreadyExists(err) {
		return mutex, false, nil
	}

	// Catch any other errors and pass them up
	if err != nil {
		return nil, false, err
	}

	log.Debugf("Created lock for name: %s", name)

	// We created the object with no issue (we have the mutex)
	return mutex, true, nil
}

// removeExpiredMutex removes a mutex object from Kubernetes if
// it is expired. It uses the object UUID to ensure no race condition
// where an unexpired lock is deleted
func (m *Mutex) removeExpiredMutex(name string) (err error) {
	// Object exists, check if its expired
	mutex, err := m.client.Get(name, metav1.GetOptions{})

	// If object is now gone then try again on the next loop
	if errors.IsNotFound(err) {
		return nil
	}

	// Catch any other errors and pass them up
	if err != nil {
		return err
	}

	// If lock is not expired then there is nothing to do
	if !m.isExpired(mutex) {
		return nil
	}

	log.Debugf("Deleting expired lock for name: %s", name)

	// If expired try delete (ignore error)
	m.client.Delete(name, &metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{
			UID: &mutex.ObjectMeta.UID,
		},
	})

	return
}

// isExpired checks to see if a mutex is expired
func (m *Mutex) isExpired(mutex *v1.Mutex) bool {
	return mutex.Expires < Now().Unix()
}

// keepOwnership updates the expiry of a mutex continuously until done is closed
// in order to keep ownership of the lock
func (m *Mutex) keepOwnership(mutex *v1.Mutex, done chan struct{}) {
	name := mutex.ObjectMeta.Name
	uid := mutex.ObjectMeta.UID
	err := wait.PollUntil(updateInterval, func() (done bool, err error) {
		log.Debugf("Attempting lock expiry update: %s", mutex.ObjectMeta.Name)

		// Get mutex object
		mutex, err := m.client.Get(name, metav1.GetOptions{})

		// Object has gone
		// this should not happen but lets not leave zombie goroutines if it does
		if errors.IsNotFound(err) {
			return false, err
		}

		// UID has changed, this is not the lock we are keeping alive
		// this should not happen but lets not leave zombie goroutines if it does
		if mutex.ObjectMeta.UID != uid {
			return false, fmt.Errorf("object uid has changed: expected %s, got %s", uid, mutex.ObjectMeta.UID)
		}

		// Generic error, try and continue
		if err != nil {
			log.Warnf("Error getting Mutex object for expiry update: %s", err)
			return false, nil
		}

		// Update object with new expiry
		clone := mutex.DeepCopy()
		clone.Expires = Now().Add(leaseDuration).Unix()
		_, err = m.client.Update(clone)

		// Generic error, try and continue
		if err != nil {
			log.Warnf("Error updating Mutex expiry: %s", err)
			return false, nil
		}

		// Poll forever (until done is closed anyway)
		return false, nil
	}, done)

	// We use PollUntil to break on error, not for timeouts, so ignore timeouts
	if err == wait.ErrWaitTimeout {
		err = nil
	}

	// Log any errors, but as this is run in a goroutine not much we can do
	if err != nil {
		log.Warnf("Error keeping mutex lock: %s", err)
		return
	}

	// Delete lock
	err = m.client.Delete(name, &metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{
			UID: &uid,
		},
	})

	// Log any errors, but as this is run in a goroutine not much we can do
	if err != nil {
		log.Warnf("Error deleting mutex lock: %s", err)
	}

	log.Infof("Released lock: %s", mutex.ObjectMeta.Name)
}
