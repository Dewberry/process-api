package jobs

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"

	"github.com/labstack/gommon/log"
)

type JobsCache struct {
	Jobs             map[string]*Job `json:"jobs"`
	MaxSizeBytes     uint64          `json:"maxCacheBytes"`
	TrimThreshold    float64         `json:"cacheTrimThreshold"`
	CurrentSizeBytes uint64          `json:"currentCacheBytes"`
	mu               sync.Mutex
}

func (jc *JobsCache) Add(j *Job) {
	jc.mu.Lock()
	defer jc.mu.Unlock()
	jc.Jobs[(*j).JobID()] = j
}

func (jc *JobsCache) Remove(j *Job) {
	jc.mu.Lock()
	defer jc.mu.Unlock()

	delete(jc.Jobs, (*j).JobID())
}

func (jc *JobsCache) DumpCacheToFile() error {
	// create a file to write the serialized data to
	err := os.MkdirAll(".data", os.ModePerm)
	if err != nil {
		return err
	}
	file, err := os.Create(".data/snapshot.gob.tmp")
	if err != nil {
		return err
	}
	defer file.Close()

	gob.Register(&DockerJob{})
	gob.Register(&AWSBatchJob{})

	// create an encoder and use it to serialize the map to the file
	encoder := gob.NewEncoder(file)
	err = encoder.Encode(jc.Jobs)
	if err != nil {
		return err
	}
	file.Close()
	// saving it to tmp is better because
	// if the gob panics then the existing snapshot is still untouched
	err = os.Rename(".data/snapshot.gob.tmp", ".data/snapshot.gob")
	if err != nil {
		return fmt.Errorf("error moving file: %v", err.Error())
	}

	return nil
}

// LoadCacheFromFile loads snapshot if it exists.
// Returns error if file could not be desearilized or not found.
// Only modifies the JobsCache if file is read and parsed correctly.
func (jc *JobsCache) LoadCacheFromFile() error {

	// create a file to read the serialized data from
	file, err := os.Open(".data/snapshot.gob")
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("not found")
	}

	js := make(map[string]*Job) //

	defer file.Close()

	gob.Register(&DockerJob{})
	gob.Register(&AWSBatchJob{})

	// create a decoder and use it to deserialize the people map from the file
	decoder := gob.NewDecoder(file)
	err = decoder.Decode(&js)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	jc.Jobs = *&js
	return nil
}

func (jc *JobsCache) TrimCache(desiredLength int64) {
	jc.mu.Lock()
	defer jc.mu.Unlock()

	jobIDs := make([]string, len(jc.Jobs))

	var i int
	for k := range jc.Jobs {
		jobIDs[i] = k
		i++
	}

	// sort the jobIDs in reverse order with most recent time first
	sort.Slice(jobIDs, func(i, j int) bool {
		return (*jc.Jobs[jobIDs[i]]).LastUpdate().After((*jc.Jobs[jobIDs[j]]).LastUpdate())
	})

	// delete these records from the map
	for _, jid := range jobIDs[0:desiredLength] {
		delete(jc.Jobs, jid)
	}
}

// Revised to kill only currently active jobs
func (jc *JobsCache) KillAll() error {
	jc.mu.Lock()
	defer jc.mu.Unlock()

	for _, j := range jc.Jobs {
		if (*j).CurrentStatus() == ACCEPTED || (*j).CurrentStatus() == RUNNING {
			if err := (*j).Kill(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (jc *JobsCache) CheckCache() uint64 {
	// jc.mu.Lock()
	// defer jc.mu.Unlock()

	// calculate total size of cache as of now
	var currentSizeBytes uint64
	for _, j := range jc.Jobs {
		currentSizeBytes += uint64((*j).GetSizeinCache())
	}
	jc.CurrentSizeBytes = currentSizeBytes

	pctCacheFull := float64(jc.CurrentSizeBytes) / float64(jc.MaxSizeBytes)
	log.Info("cache_pct_full=", pctCacheFull, " current_size=", float64(jc.CurrentSizeBytes), " jobs=", len(jc.Jobs), " (max cache=", float64(jc.MaxSizeBytes), ")")
	// set default auto-trim to 95%....
	if pctCacheFull > 0.95 {
		currentLenth := len(jc.Jobs)
		desiredLength := int64(jc.TrimThreshold * float64(currentLenth))
		message := fmt.Sprintf("trimming cache from %d jobs to %d jobs", currentLenth, desiredLength)
		log.Info(message)
		jc.TrimCache(desiredLength)
	}
	return currentSizeBytes
}
